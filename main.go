package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const requestTimeout = time.Second

type Address struct {
	CEP          string `json:"cep"`
	Street       string `json:"logradouro"`
	Complement   string `json:"complemento,omitempty"`
	Neighborhood string `json:"bairro"`
	City         string `json:"localidade"`
	State        string `json:"uf"`
}

type apiResult struct {
	API     string
	Address Address
	Err     error
}

type lookupFunc func(context.Context, string) (Address, error)

var cepPattern = regexp.MustCompile(`^\d{8}$`)

func normalizeCEP(cep string) (string, error) {
	cep = strings.TrimSpace(cep)
	cep = strings.ReplaceAll(cep, "-", "")

	if !cepPattern.MatchString(cep) {
		return "", errors.New("CEP inválido: informe 8 números, com ou sem hífen")
	}

	return cep, nil
}

func fetchAddress(ctx context.Context, client *http.Client, endpoint string, cep string, isViaCEP bool) (Address, error) {
	requestURL := fmt.Sprintf(endpoint, url.PathEscape(cep))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return Address{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return Address{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Address{}, fmt.Errorf("a API respondeu com status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Address{}, err
	}

	if isViaCEP {
		var result struct {
			CEP          string          `json:"cep"`
			Street       string          `json:"logradouro"`
			Complement   string          `json:"complemento"`
			Neighborhood string          `json:"bairro"`
			City         string          `json:"localidade"`
			State        string          `json:"uf"`
			Erro         json.RawMessage `json:"erro"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return Address{}, fmt.Errorf("não foi possível interpretar a resposta da ViaCEP: %w", err)
		}

		isNotFound := false
		if len(result.Erro) > 0 {
			var b bool
			if err := json.Unmarshal(result.Erro, &b); err == nil && b {
				isNotFound = true
			}
			var s string
			if err := json.Unmarshal(result.Erro, &s); err == nil && (s == "true" || s == "1") {
				isNotFound = true
			}
		}

		if isNotFound || result.CEP == "" {
			return Address{}, errors.New("CEP não encontrado pela ViaCEP")
		}
		return Address{
			CEP: result.CEP, Street: result.Street, Complement: result.Complement,
			Neighborhood: result.Neighborhood, City: result.City, State: result.State,
		}, nil
	}

	var result struct {
		CEP          string `json:"cep"`
		Street       string `json:"street"`
		Neighborhood string `json:"neighborhood"`
		City         string `json:"city"`
		State        string `json:"state"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return Address{}, fmt.Errorf("não foi possível interpretar a resposta da BrasilAPI: %w", err)
	}
	if result.CEP == "" {
		return Address{}, errors.New("CEP não encontrado pela BrasilAPI")
	}

	return Address{
		CEP: result.CEP, Street: result.Street,
		Neighborhood: result.Neighborhood, City: result.City, State: result.State,
	}, nil
}

func lookupBrasilAPI(ctx context.Context, client *http.Client, cep string) (Address, error) {
	return fetchAddress(ctx, client, "https://brasilapi.com.br/api/cep/v1/%s", cep, false)
}

func lookupViaCEP(ctx context.Context, client *http.Client, cep string) (Address, error) {
	return fetchAddress(ctx, client, "https://viacep.com.br/ws/%s/json/", cep, true)
}

// findFastest returns the first valid address received before the one-second deadline.
// Each API runs in its own goroutine and reports its result through a buffered channel.
func findFastest(
	ctx context.Context,
	cep string,
	timeout time.Duration,
	brasil lookupFunc,
	via lookupFunc,
) (Address, string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	results := make(chan apiResult, 2)

	go func() {
		address, err := brasil(ctx, cep)
		results <- apiResult{API: "BrasilAPI", Address: address, Err: err}
	}()
	go func() {
		address, err := via(ctx, cep)
		results <- apiResult{API: "ViaCEP", Address: address, Err: err}
	}()

	var errorsSeen []string
	for received := 0; received < 2; received++ {
		select {
		case result := <-results:
			if result.Err == nil && result.Address.CEP != "" {
				return result.Address, result.API, nil
			}
			if result.Err != nil {
				errorsSeen = append(errorsSeen, result.API+": "+result.Err.Error())
			} else {
				errorsSeen = append(errorsSeen, result.API+": resposta sem endereço válido")
			}
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return Address{}, "", fmt.Errorf("timeout: nenhuma API retornou um endereço válido em %s", timeout)
			}
			return Address{}, "", ctx.Err()
		}
	}

	return Address{}, "", fmt.Errorf("nenhuma API retornou um endereço válido (%s)", strings.Join(errorsSeen, "; "))
}

func printAddress(address Address, api string) {
	fmt.Println("\nEndereço encontrado:")
	fmt.Printf("API vencedora: %s\n", api)
	fmt.Printf("CEP: %s\n", address.CEP)
	fmt.Printf("Logradouro: %s\n", valueOrDash(address.Street))
	fmt.Printf("Complemento: %s\n", valueOrDash(address.Complement))
	fmt.Printf("Bairro: %s\n", valueOrDash(address.Neighborhood))
	fmt.Printf("Cidade: %s\n", valueOrDash(address.City))
	fmt.Printf("UF: %s\n", valueOrDash(address.State))
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("busca-cep-go", flag.ContinueOnError)
	flags.SetOutput(stderr)
	cepFlag := flags.String("cep", "", "CEP para consultar (8 números)")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	cep := *cepFlag
	if cep == "" && flags.NArg() > 0 {
		cep = flags.Arg(0)
	}
	if cep == "" {
		fmt.Fprintln(stderr, "Uso: go run . --cep=01001000")
		return 2
	}

	normalized, err := normalizeCEP(cep)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	client := &http.Client{}
	ctx := context.Background()

	address, api, err := findFastest(
		ctx,
		normalized,
		requestTimeout,
		func(ctx context.Context, cep string) (Address, error) {
			return lookupBrasilAPI(ctx, client, cep)
		},
		func(ctx context.Context, cep string) (Address, error) {
			return lookupViaCEP(ctx, client, cep)
		},
	)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintln(stdout, "Consulta concluída.")
	printAddressTo(stdout, address, api)
	return 0
}

func printAddressTo(out io.Writer, address Address, api string) {
	fmt.Fprintln(out, "\nEndereço encontrado:")
	fmt.Fprintf(out, "API vencedora: %s\n", api)
	fmt.Fprintf(out, "CEP: %s\n", address.CEP)
	fmt.Fprintf(out, "Logradouro: %s\n", valueOrDash(address.Street))
	fmt.Fprintf(out, "Complemento: %s\n", valueOrDash(address.Complement))
	fmt.Fprintf(out, "Bairro: %s\n", valueOrDash(address.Neighborhood))
	fmt.Fprintf(out, "Cidade: %s\n", valueOrDash(address.City))
	fmt.Fprintf(out, "UF: %s\n", valueOrDash(address.State))
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
