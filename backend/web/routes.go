// Package web é a camada HTTP do UchôaStock: rotas, handlers, sessão,
// autorização e a montagem das telas. Não escreve SQL: os dados vêm de
// backend/services.
package web

import "net/http"

// NewHandler monta o roteador com todas as rotas, já protegido contra
// CSRF. É o que o servidor do main atende.
func NewHandler() http.Handler {
	mux := http.NewServeMux()

	static := staticHandler()
	mux.Handle("/css/", static)
	mux.Handle("/js/", static)
	mux.Handle("/images/", static)

	mux.HandleFunc("/", indexHandler)
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/logout", logoutHandler)

	mux.HandleFunc("/dashboard", withUser(dashboardHandler))

	mux.HandleFunc("/obras", withUser(siteHandler))
	mux.HandleFunc("/obra-atual", withUser(siteSwitchHandler))

	mux.HandleFunc("/materiais", withUser(materialHandler))
	mux.HandleFunc("/alterar-material", withUser(editMaterialHandler))

	mux.HandleFunc("/estoque", withUser(stockHandler))

	mux.HandleFunc("/fornecedores", withUser(supplierHandler))
	mux.HandleFunc("/patrimonio", withUser(assetListHandler))
	mux.HandleFunc("/patrimonio/{id}", withUser(assetDetailHandler))

	mux.HandleFunc("/movimentacoes", withUser(movementHandler))
	mux.HandleFunc("/movimentacoes/exportar", withUser(movementExportHandler))

	mux.HandleFunc("/relatorios", withUser(reportHandler))

	mux.HandleFunc("/inventarios", withUser(inventoryListHandler))
	mux.HandleFunc("/inventarios/{id}", withUser(inventoryDetailHandler))
	mux.HandleFunc("/inventarios/{id}/exportar", withUser(inventoryExportHandler))

	// "/solicitacoes/nova" é mais específica que "/solicitacoes/{id}": o
	// roteador do Go escolhe a mais específica, então "nova" nunca vira ID.
	mux.HandleFunc("/solicitacoes", withUser(requestListHandler))
	mux.HandleFunc("/solicitacoes/nova", withUser(newRequestHandler))
	mux.HandleFunc("/solicitacoes/{id}", withUser(requestDetailHandler))

	// A tela já se chamou "/requisicoes". Link antigo — favorito, mensagem
	// no grupo da obra, aba esquecida aberta — continua funcionando: o
	// redirect leva para o endereço novo sem perder a query string.
	mux.HandleFunc("/requisicoes", redirectToRequests)
	mux.HandleFunc("/requisicoes/{rest...}", redirectToRequests)

	mux.HandleFunc("/usuarios", withUser(userHandler))
	mux.HandleFunc("/minha-senha", withUser(myPasswordHandler))

	return securityHeaders(csrfProtection().Handler(mux))
}

// securityHeaders acrescenta a toda resposta cabeçalhos que pedem ao
// navegador para se proteger:
//   - frame-ancestors 'none' e X-Frame-Options DENY: nenhum site pode
//     abrir o UchôaStock dentro de um <iframe>. Sem isso, uma página
//     maliciosa podia carregar o sistema invisível por cima de um botão
//     dela e fazer a pessoa clicar em "Remover" sem saber (clickjacking).
//     A proteção de CSRF não pega esse caso, porque o clique acontece na
//     página verdadeira. O X-Frame-Options é o mesmo aviso para
//     navegadores antigos.
//   - nosniff: o navegador usa o tipo que o servidor declarou (CSS é CSS)
//     em vez de "adivinhar" pelo conteúdo do arquivo.
//   - Referrer-Policy same-origin: ao clicar num link para fora, o
//     endereço da tela (que pode ter busca e ID na query string) não vai
//     junto para o outro site.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("Content-Security-Policy", "frame-ancestors 'none'")
		header.Set("X-Frame-Options", "DENY")
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// csrfProtection barra POST que vem de outro site (CSRF): uma página
// qualquer, aberta no mesmo navegador de quem está logado, não consegue
// mandar um formulário para o UchôaStock usando a sessão dessa pessoa.
// O navegador informa de onde veio o pedido (Sec-Fetch-Site / Origin), e
// só pedido do próprio site passa. GET não é conferido, por isso nenhum
// GET pode alterar dados.
func csrfProtection() *http.CrossOriginProtection {
	protection := http.NewCrossOriginProtection()
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Pedido bloqueado: ele veio de outro site. Abra o UchôaStock e tente de novo.", http.StatusForbidden)
	}))
	return protection
}
