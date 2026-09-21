package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	database "uchoastock/backend/database"
	"uchoastock/backend/services"
)

// assetCall chama o handler de detalhe como a rota /patrimonio/{id}
// chamaria, com a sessão do token.
func assetCall(token string, assetID int, form url.Values) *httptest.ResponseRecorder {
	method, body := http.MethodGet, ""
	if form != nil {
		method, body = http.MethodPost, form.Encode()
	}
	request := httptest.NewRequest(method, fmt.Sprintf("/patrimonio/%d", assetID), strings.NewReader(body))
	if form != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	request.SetPathValue("id", fmt.Sprint(assetID))
	request.AddCookie(&http.Cookie{Name: "sessao", Value: token})
	recorder := httptest.NewRecorder()
	withUser(assetDetailHandler).ServeHTTP(recorder, request)
	return recorder
}

// assetState copia, como texto, a obra e a situação de todos os bens e o
// histórico inteiro. Se o texto não muda, nada foi gravado.
func assetState(t *testing.T) string {
	t.Helper()
	var out strings.Builder
	for _, query := range []string{
		`SELECT id, numero, obra_id, situacao FROM patrimonios ORDER BY id`,
		`SELECT id, patrimonio_id, acao, COALESCE(obra_destino_id, 0) FROM patrimonio_movimentacoes ORDER BY id`,
	} {
		rows, err := database.DB.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var a, b, c, d string
			if err := rows.Scan(&a, &b, &c, &d); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintln(&out, a, b, c, d)
		}
		rows.Close()
	}
	return out.String()
}

// TestAssetPermissionsOverHTTP: o gestor e o almoxarife da obra A tentam
// mexer num bem da obra B pelo endereço direto; o almoxarife tenta
// cadastrar e reativar, que são só do administrador. Tudo é recusado e o
// banco não muda. Depois o almoxarife transfere um bem da própria obra.
func TestAssetPermissionsOverHTTP(t *testing.T) {
	f := setupIsolation(t)
	adminID := queryID(t, `SELECT id FROM usuarios WHERE email = 'admin.teste@empresa.com'`)

	assetA, err := services.CreateAsset(adminID, "PAT-A", "Furadeira", "", f.siteA)
	if err != nil {
		t.Fatal(err)
	}
	assetB, err := services.CreateAsset(adminID, "PAT-B", "Betoneira", "", f.siteB)
	if err != nil {
		t.Fatal(err)
	}
	retiredA, err := services.CreateAsset(adminID, "PAT-X", "Serra", "", f.siteA)
	if err != nil {
		t.Fatal(err)
	}
	if err := services.ChangeAssetStatus(adminID, retiredA, f.siteA, services.AssetStatusRetired, "quebrou", true); err != nil {
		t.Fatal(err)
	}

	before := assetState(t)
	attacks := []struct {
		name  string
		token string
		id    int
		form  url.Values
	}{
		{"gestor A transfere bem da obra B", f.managerToken, assetB,
			url.Values{"acao": {"transferir"}, "obra_atual": {fmt.Sprint(f.siteB)}, "obra_destino": {fmt.Sprint(f.siteA)}}},
		{"almoxarife A dá baixa em bem da obra B", f.storekeeperToken, assetB,
			url.Values{"acao": {"situacao"}, "obra_atual": {fmt.Sprint(f.siteB)}, "situacao": {"BAIXADO"}, "observacao": {"x"}}},
		{"almoxarife A edita bem", f.storekeeperToken, assetA,
			url.Values{"acao": {"editar"}, "numero": {"PAT-Z"}, "nome": {"Outro"}}},
		{"almoxarife A reativa bem baixado", f.storekeeperToken, retiredA,
			url.Values{"acao": {"situacao"}, "obra_atual": {fmt.Sprint(f.siteA)}, "situacao": {"EM_USO"}}},
	}
	for _, a := range attacks {
		if got := assetCall(a.token, a.id, a.form).Code; got != http.StatusForbidden {
			t.Errorf("%s: status %d, esperado 403", a.name, got)
		}
	}

	create := post(f.storekeeperToken, assetListHandler, "/patrimonio",
		url.Values{"acao": {"cadastrar"}, "numero": {"PAT-NOVO"}, "nome": {"Martelete"}, "obra": {fmt.Sprint(f.siteA)}})
	if create.Code != http.StatusForbidden {
		t.Errorf("almoxarife cadastrando bem: status %d, esperado 403", create.Code)
	}

	// Tela velha: o bem estava na obra B quando a página abriu, mas a
	// pessoa manda obra_atual errada. Recusa com aviso, sem gravar.
	stale := assetCall(f.adminToken, assetA, url.Values{"acao": {"transferir"}, "obra_atual": {fmt.Sprint(f.siteB)}, "obra_destino": {fmt.Sprint(f.siteB)}})
	if stale.Code != http.StatusBadRequest || !strings.Contains(stale.Body.String(), "mudou de obra") {
		t.Errorf("tela velha: status %d, esperado 400 com aviso", stale.Code)
	}

	if after := assetState(t); after != before {
		t.Errorf("o banco mudou depois das tentativas recusadas:\nantes:\n%s\ndepois:\n%s", before, after)
	}

	// Qualquer cargo vê o bem, mesmo de outra obra; só não vê os botões.
	view := assetCall(f.storekeeperToken, assetB, nil)
	if view.Code != http.StatusOK || strings.Contains(view.Body.String(), `value="transferir"`) {
		t.Errorf("almoxarife A vendo bem da obra B: status %d, botões de ação na tela", view.Code)
	}

	// Na própria obra, o almoxarife transfere.
	ok := assetCall(f.storekeeperToken, assetA, url.Values{"acao": {"transferir"}, "obra_atual": {fmt.Sprint(f.siteA)}, "obra_destino": {fmt.Sprint(f.siteB)}})
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("almoxarife transferindo bem da própria obra: status %d, corpo %s", ok.Code, ok.Body.String())
	}
	asset, err := services.GetAssetByID(assetA)
	if err != nil || asset.SiteID != f.siteB {
		t.Errorf("bem deveria estar na obra B: %+v, %v", asset, err)
	}

	// Bem inexistente: 404, como qualquer endereço que não existe.
	if got := assetCall(f.adminToken, 9999, nil).Code; got != http.StatusNotFound {
		t.Errorf("bem inexistente: status %d, esperado 404", got)
	}
}

// TestAssetListAndCreateOverHTTP: o administrador cadastra pela lista e é
// levado ao detalhe; a lista mostra o bem.
func TestAssetListAndCreateOverHTTP(t *testing.T) {
	f := setupIsolation(t)

	created := post(f.adminToken, assetListHandler, "/patrimonio",
		url.Values{"acao": {"cadastrar"}, "numero": {"pat-9"}, "nome": {"Andaime"}, "obra": {fmt.Sprint(f.siteA)}})
	if created.Code != http.StatusSeeOther || !strings.HasPrefix(created.Header().Get("Location"), "/patrimonio/") {
		t.Fatalf("cadastro: status %d, destino %q", created.Code, created.Header().Get("Location"))
	}

	repeated := post(f.adminToken, assetListHandler, "/patrimonio",
		url.Values{"acao": {"cadastrar"}, "numero": {"PAT-9"}, "nome": {"Outro"}, "obra": {fmt.Sprint(f.siteA)}})
	if repeated.Code != http.StatusBadRequest || !strings.Contains(repeated.Body.String(), "já existe um bem com o número PAT-9") {
		t.Errorf("número repetido: status %d, esperado 400 com a mensagem", repeated.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/patrimonio", nil)
	request.AddCookie(&http.Cookie{Name: "sessao", Value: f.managerToken})
	list := httptest.NewRecorder()
	withUser(assetListHandler).ServeHTTP(list, request)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "PAT-9") {
		t.Errorf("lista do gestor A: status %d, esperado o PAT-9", list.Code)
	}
	if strings.Contains(list.Body.String(), "Novo bem") {
		t.Error("o gestor não deveria ver o botão Novo bem")
	}
}
