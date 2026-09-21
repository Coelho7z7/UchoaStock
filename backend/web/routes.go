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

	return csrfProtection().Handler(mux)
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
