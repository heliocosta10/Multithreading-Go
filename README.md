# Busca CEP Go

Projeto em Go que consulta duas APIs de CEP ao mesmo tempo e apresenta o primeiro endereço válido recebido. A ideia é praticar goroutines, channels, `select`, concorrência e timeout usando os recursos nativos do Go.

## APIs utilizadas

- **BrasilAPI:** `https://brasilapi.com.br/api/cep/v1/{cep}`
- **ViaCEP:** `https://viacep.com.br/ws/{cep}/json/`

## Como funciona

1. A aplicação valida o CEP informado.
2. Duas goroutines iniciam as consultas em paralelo.
3. Os resultados são enviados para um channel com buffer.
4. O `select` recebe os resultados e retorna o primeiro endereço válido.
5. O contexto cancela as requisições quando o prazo de 1 segundo termina.
6. O terminal mostra o endereço e qual API venceu.

Se uma API falhar ou não encontrar o CEP, a aplicação ainda espera pela outra dentro do prazo. Se nenhuma API retornar um endereço válido antes do limite, é exibido um erro de timeout.

## Requisitos

- Go 1.22 ou superior
- Conexão com a internet para consultar as APIs reais

## Como executar

Clone o repositório e entre na pasta:

```bash
git clone https://github.com/heliocosta10/Multithreading-Go.git
cd Multithreading-Go
```

Execute informando o CEP:

```bash
go run . --cep=01001000
```

Também é possível usar o CEP como argumento posicional:

```bash
go run . 01001-000
```

Exemplo de saída:

```text
Consulta concluída.

Endereço encontrado:
API vencedora: ViaCEP
CEP: 01001-000
Logradouro: Praça da Sé
Complemento: lado ímpar
Bairro: Sé
Cidade: São Paulo
UF: SP
```

A API vencedora e alguns campos do endereço podem variar de acordo com a resposta recebida.

## Como testar

Execute os testes automatizados:

```bash
go test ./...
```

Para ver o nome de cada teste:

```bash
go test -v ./...
```

Os testes cobrem:

- Validação do CEP com e sem hífen e rejeição de entradas inválidas.
- Escolha da API que retorna primeiro um endereço válido.
- Uso da outra API quando a primeira falha.
- Timeout quando nenhuma API responde a tempo.
- Leitura e interpretação de uma resposta simulada da ViaCEP.
- Tratamento de CEP não encontrado.
- Conteúdo exibido no terminal.

Os testes usam `httptest` e funções simuladas para validar os cenários sem depender da disponibilidade das APIs externas.

## Tecnologias e conceitos

- Go (Golang)
- `net/http`
- Goroutines
- Channels
- `select`
- `context.WithTimeout`
- Testes com `testing` e `httptest`

## Observação sobre o timeout

O prazo de 1 segundo vale para a espera pelo primeiro endereço válido. A aplicação cancela o contexto compartilhado quando recebe uma resposta válida ou quando o prazo termina. Como acontece em sistemas concorrentes, uma requisição já enviada pode ter chegado ao servidor externo antes do cancelamento.

## Autor

Hélio Costa
