package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizeCEP(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "CEP sem hífen", input: "01001000", want: "01001000"},
		{name: "CEP com hífen", input: "01001-000", want: "01001000"},
		{name: "CEP com espaços", input: " 01001000 ", want: "01001000"},
		{name: "CEP curto", input: "123", wantErr: true},
		{name: "letras", input: "abcdefgh", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeCEP(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeCEP() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("normalizeCEP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindFastestReturnsFirstValidAPI(t *testing.T) {
	slow := func(ctx context.Context, cep string) (Address, error) {
		select {
		case <-time.After(150 * time.Millisecond):
			return Address{CEP: cep, City: "Cidade lenta"}, nil
		case <-ctx.Done():
			return Address{}, ctx.Err()
		}
	}
	fast := func(ctx context.Context, cep string) (Address, error) {
		select {
		case <-time.After(10 * time.Millisecond):
			return Address{CEP: cep, City: "Cidade rápida"}, nil
		case <-ctx.Done():
			return Address{}, ctx.Err()
		}
	}

	address, api, err := findFastest(context.Background(), "01001000", time.Second, fast, slow)
	if err != nil {
		t.Fatalf("esperava sucesso, recebeu: %v", err)
	}
	if api != "BrasilAPI" {
		t.Fatalf("API vencedora = %q, esperava BrasilAPI", api)
	}
	if address.City != "Cidade rápida" {
		t.Fatalf("endereço inesperado: %+v", address)
	}
}

func TestFindFastestUsesOtherAPIWhenFirstFails(t *testing.T) {
	fails := func(ctx context.Context, cep string) (Address, error) {
		return Address{}, errors.New("falha simulada")
	}
	succeeds := func(ctx context.Context, cep string) (Address, error) {
		return Address{CEP: cep, City: "São Paulo"}, nil
	}

	address, api, err := findFastest(context.Background(), "01001000", time.Second, fails, succeeds)
	if err != nil {
		t.Fatalf("esperava que a segunda API funcionasse: %v", err)
	}
	if api != "ViaCEP" || address.City != "São Paulo" {
		t.Fatalf("resultado inesperado: API=%s, endereço=%+v", api, address)
	}
}

func TestFindFastestTimeout(t *testing.T) {
	slow := func(ctx context.Context, cep string) (Address, error) {
		<-ctx.Done()
		return Address{}, ctx.Err()
	}

	start := time.Now()
	_, _, err := findFastest(context.Background(), "01001000", 50*time.Millisecond, slow, slow)
	elapsed := time.Since(start)

	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("esperava erro de timeout, recebeu: %v", err)
	}
	if elapsed > 300*time.Millisecond {
		t.Fatalf("timeout demorou mais que o esperado: %s", elapsed)
	}
}

func TestFetchAddressViaCEP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"cep":"01001-000","logradouro":"Praça da Sé","complemento":"lado ímpar","bairro":"Sé","localidade":"São Paulo","uf":"SP"}`)
	}))
	defer server.Close()

	client := server.Client()
	address, err := fetchAddress(context.Background(), client, server.URL+"/%s", "01001000", true)
	if err != nil {
		t.Fatalf("fetchAddress() retornou erro: %v", err)
	}
	if address.CEP != "01001-000" || address.City != "São Paulo" || address.State != "SP" {
		t.Fatalf("endereço inesperado: %+v", address)
	}
}

func TestViaCEPNotFound(t *testing.T) {
	for _, body := range []string{`{"erro": true}`, `{"erro": "true"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, body)
		}))

		_, err := fetchAddress(context.Background(), server.Client(), server.URL+"/%s", "99999999", true)
		server.Close()
		if err == nil || !strings.Contains(err.Error(), "não encontrado") {
			t.Fatalf("com payload %s, esperava erro de CEP não encontrado, recebeu: %v", body, err)
		}
	}
}


func TestPrintAddressTo(t *testing.T) {
	var output strings.Builder
	printAddressTo(&output, Address{
		CEP: "01001-000", Street: "Praça da Sé", Neighborhood: "Sé",
		City: "São Paulo", State: "SP",
	}, "ViaCEP")

	got := output.String()
	for _, expected := range []string{"ViaCEP", "01001-000", "Praça da Sé", "São Paulo", "SP"} {
		if !strings.Contains(got, expected) {
			t.Errorf("saída não contém %q: %s", expected, got)
		}
	}
}

func ExamplenormalizeCEP() {
	cep, _ := normalizeCEP("01001-000")
	fmt.Println(cep)
	// Output: 01001000
}
