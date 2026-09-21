package services

import (
	"errors"
	"strings"
	"testing"

	database "uchoastock/backend/database"
)

// assertAssetInputError confere que err é um erro de digitação do
// patrimônio (e não do banco) e que a mensagem contém o trecho esperado.
func assertAssetInputError(t *testing.T, err error, contains string) {
	t.Helper()
	var inputErr AssetInputError
	if !errors.As(err, &inputErr) {
		t.Fatalf("esperava AssetInputError com %q, veio %v", contains, err)
	}
	if !strings.Contains(inputErr.Message, contains) {
		t.Errorf("mensagem = %q, esperado conter %q", inputErr.Message, contains)
	}
}

// createTestAsset cadastra um bem e devolve o ID.
func createTestAsset(t *testing.T, userID int, number string, siteID int) int {
	t.Helper()
	id, err := CreateAsset(userID, number, "Betoneira "+number, "", siteID)
	if err != nil {
		t.Fatalf("cadastrar bem %s: %v", number, err)
	}
	return id
}

// assetEventActions devolve as ações do histórico do bem, da mais antiga
// para a mais nova.
func assetEventActions(t *testing.T, assetID int) []string {
	t.Helper()
	rows, err := database.DB.Query(`SELECT acao FROM patrimonio_movimentacoes WHERE patrimonio_id = ? ORDER BY id`, assetID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var actions []string
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			t.Fatal(err)
		}
		actions = append(actions, action)
	}
	return actions
}

func TestCreateAssetNormalizesAndRecordsHistory(t *testing.T) {
	userID := setupTestDB(t)
	site := createTestSite(t, "Obra Norte", SiteStatusInProgress)

	id, err := CreateAsset(userID, "  pat-0142 ", " Betoneira 400 L ", " Marca X ", site)
	if err != nil {
		t.Fatalf("cadastro falhou: %v", err)
	}

	asset, err := GetAssetByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if asset.Number != "PAT-0142" || asset.Name != "Betoneira 400 L" || asset.Description != "Marca X" {
		t.Errorf("campos não normalizados: %q / %q / %q", asset.Number, asset.Name, asset.Description)
	}
	if asset.SiteID != site || asset.SiteName != "Obra Norte" {
		t.Errorf("obra = %d (%s), esperado %d", asset.SiteID, asset.SiteName, site)
	}
	if asset.Status != AssetStatusInUse || asset.FormattedStatus != "Em uso" {
		t.Errorf("situação = %s (%s), esperado EM_USO", asset.Status, asset.FormattedStatus)
	}

	events, err := GetAssetEvents(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Action != AssetEventCreated || events[0].ToSiteName != "Obra Norte" {
		t.Errorf("histórico do cadastro = %+v", events)
	}
}

func TestCreateAssetRejectsInvalidInput(t *testing.T) {
	userID := setupTestDB(t)
	site := createTestSite(t, "Obra Norte", SiteStatusInProgress)
	finished := createTestSite(t, "Obra Antiga", SiteStatusFinished)
	createTestAsset(t, userID, "PAT-1", site)

	cases := []struct {
		number, name string
		site         int
		want         string
	}{
		{"  ", "Furadeira", site, "número de patrimônio"},
		{"PAT-2", "   ", site, "nome do bem"},
		// Mesmo número com outra caixa: é a mesma plaqueta.
		{"pat-1", "Furadeira", site, "já existe"},
		{strings.Repeat("9", maxAssetNumberLength+1), "Furadeira", site, "no máximo"},
		{"PAT-3", strings.Repeat("ã", maxAssetNameLength+1), site, "no máximo"},
		{"PAT-4", "Furadeira", 0, "escolha a obra"},
		{"PAT-5", "Furadeira", 9999, "escolha a obra"},
		{"PAT-6", "Furadeira", finished, "concluída"},
	}
	for _, c := range cases {
		_, err := CreateAsset(userID, c.number, c.name, "", c.site)
		assertAssetInputError(t, err, c.want)
	}

	var count int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM patrimonios`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("%d bens no banco, esperado só o primeiro", count)
	}
}

func TestTransferAsset(t *testing.T) {
	userID := setupTestDB(t)
	north := createTestSite(t, "Obra Norte", SiteStatusInProgress)
	south := createTestSite(t, "Obra Sul", SiteStatusInProgress)
	id := createTestAsset(t, userID, "PAT-1", north)

	if err := TransferAsset(userID, id, north, south, "  no caminhão  "); err != nil {
		t.Fatalf("transferência falhou: %v", err)
	}
	asset, _ := GetAssetByID(id)
	if asset.SiteID != south {
		t.Fatalf("bem ficou na obra %d, esperado %d", asset.SiteID, south)
	}

	events, _ := GetAssetEvents(id)
	last := events[0]
	if last.Action != AssetEventTransfer || last.FromSiteName != "Obra Norte" || last.ToSiteName != "Obra Sul" || last.Note != "no caminhão" {
		t.Errorf("evento da transferência = %+v", last)
	}

	// A tela ainda mostrava o bem na obra Norte: recusa, sem mexer em nada.
	if err := TransferAsset(userID, id, north, north, ""); !errors.Is(err, ErrAssetMoved) {
		t.Errorf("transferir de obra antiga: esperado ErrAssetMoved, veio %v", err)
	}
	assertAssetInputError(t, TransferAsset(userID, id, south, south, ""), "já está nesta obra")
	assertAssetInputError(t, TransferAsset(userID, id, south, 9999, ""), "escolha a obra")
	if err := TransferAsset(userID, 9999, south, north, ""); !errors.Is(err, ErrAssetNotFound) {
		t.Errorf("bem inexistente: esperado ErrAssetNotFound, veio %v", err)
	}

	if got := len(assetEventActions(t, id)); got != 2 {
		t.Errorf("%d eventos, esperado 2 (cadastro e uma transferência)", got)
	}
}

func TestTransferAssetSiteRules(t *testing.T) {
	userID := setupTestDB(t)
	north := createTestSite(t, "Obra Norte", SiteStatusInProgress)
	south := createTestSite(t, "Obra Sul", SiteStatusInProgress)
	id := createTestAsset(t, userID, "PAT-1", north)

	// Obra concluída não recebe bem...
	finished := createTestSite(t, "Obra Antiga", SiteStatusFinished)
	assertAssetInputError(t, TransferAsset(userID, id, north, finished, ""), "concluída")

	// ...mas o bem pode sair dela: é de obra encerrada que a ferramenta
	// precisa ser levada embora.
	if err := ChangeSiteStatus(north, SiteStatusFinished, false); err != nil {
		t.Fatalf("encerrar obra: %v", err)
	}
	if err := TransferAsset(userID, id, north, south, ""); err != nil {
		t.Errorf("transferir de obra concluída deveria funcionar: %v", err)
	}

	// Bem baixado não é transferido.
	if err := ChangeAssetStatus(userID, id, south, AssetStatusRetired, "quebrou", false); err != nil {
		t.Fatal(err)
	}
	other := createTestSite(t, "Obra Leste", SiteStatusInProgress)
	assertAssetInputError(t, TransferAsset(userID, id, south, other, ""), "baixado")
}

func TestChangeAssetStatus(t *testing.T) {
	userID := setupTestDB(t)
	site := createTestSite(t, "Obra Norte", SiteStatusInProgress)
	id := createTestAsset(t, userID, "PAT-1", site)

	assertAssetInputError(t, ChangeAssetStatus(userID, id, site, "QUEBRADO", "", false), "situação inválida")
	assertAssetInputError(t, ChangeAssetStatus(userID, id, site, AssetStatusInUse, "", false), "já está em uso")

	if err := ChangeAssetStatus(userID, id, site, AssetStatusMaintenance, "", false); err != nil {
		t.Fatalf("mandar para manutenção: %v", err)
	}

	// Baixa sem motivo é recusada.
	assertAssetInputError(t, ChangeAssetStatus(userID, id, site, AssetStatusRetired, "   ", false), "motivo da baixa")
	if err := ChangeAssetStatus(userID, id, site, AssetStatusRetired, "motor queimado", false); err != nil {
		t.Fatalf("dar baixa: %v", err)
	}

	// Baixado só vai para em uso, e só com permissão de reativar.
	assertAssetInputError(t, ChangeAssetStatus(userID, id, site, AssetStatusMaintenance, "", true), "não pode passar")
	if err := ChangeAssetStatus(userID, id, site, AssetStatusInUse, "", false); !errors.Is(err, ErrReactivateNotAllowed) {
		t.Errorf("reativar sem permissão: esperado ErrReactivateNotAllowed, veio %v", err)
	}
	if err := ChangeAssetStatus(userID, id, site, AssetStatusInUse, "consertado", true); err != nil {
		t.Errorf("reativar com permissão: %v", err)
	}

	want := "CADASTRO,SITUACAO,SITUACAO,SITUACAO"
	if got := strings.Join(assetEventActions(t, id), ","); got != want {
		t.Errorf("histórico = %s, esperado %s", got, want)
	}
}

func TestUpdateAsset(t *testing.T) {
	userID := setupTestDB(t)
	site := createTestSite(t, "Obra Norte", SiteStatusInProgress)
	id := createTestAsset(t, userID, "PAT-1", site)
	createTestAsset(t, userID, "PAT-2", site)

	// Salvar sem mudar nada não gera evento.
	if err := UpdateAsset(userID, id, "pat-1", "Betoneira PAT-1", ""); err != nil {
		t.Fatal(err)
	}
	if got := len(assetEventActions(t, id)); got != 1 {
		t.Errorf("%d eventos depois de salvar igual, esperado 1", got)
	}

	assertAssetInputError(t, UpdateAsset(userID, id, "PAT-2", "Betoneira", ""), "já existe")
	if err := UpdateAsset(userID, 9999, "PAT-9", "X", ""); !errors.Is(err, ErrAssetNotFound) {
		t.Errorf("bem inexistente: esperado ErrAssetNotFound, veio %v", err)
	}

	if err := UpdateAsset(userID, id, "PAT-10", "Betoneira grande", "nova"); err != nil {
		t.Fatal(err)
	}
	events, _ := GetAssetEvents(id)
	if events[0].Action != AssetEventEdit || events[0].Note != "Número: PAT-1 → PAT-10; Nome: Betoneira PAT-1 → Betoneira grande; Descrição alterada" {
		t.Errorf("evento da edição = %+v", events[0])
	}
}

func TestGetAssetsFilters(t *testing.T) {
	userID := setupTestDB(t)
	north := createTestSite(t, "Obra Norte", SiteStatusInProgress)
	south := createTestSite(t, "Obra Sul", SiteStatusInProgress)
	createTestAsset(t, userID, "PAT-1", north)
	createTestAsset(t, userID, "PAT-2", south)
	retired := createTestAsset(t, userID, "PAT-3", north)
	if err := ChangeAssetStatus(userID, retired, north, AssetStatusRetired, "perdida", false); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		site   int
		search string
		status string
		want   string
	}{
		{"padrão esconde baixados", 0, "", "", "PAT-1,PAT-2"},
		{"todas as situações", 0, "", AssetStatusAll, "PAT-1,PAT-2,PAT-3"},
		{"só baixados", 0, "", AssetStatusRetired, "PAT-3"},
		{"por obra", north, "", "", "PAT-1"},
		{"busca pelo número", 0, "pat-2", "", "PAT-2"},
	}
	for _, c := range cases {
		assets, err := GetAssets(c.site, c.search, c.status)
		if err != nil {
			t.Fatal(err)
		}
		var numbers []string
		for _, a := range assets {
			numbers = append(numbers, a.Number)
		}
		if got := strings.Join(numbers, ","); got != c.want {
			t.Errorf("%s: %s, esperado %s", c.name, got, c.want)
		}
	}
}

func TestNextAssetStatuses(t *testing.T) {
	values := func(current string, allow bool) string {
		var out []string
		for _, s := range NextAssetStatuses(current, allow) {
			out = append(out, s.Value)
		}
		return strings.Join(out, ",")
	}
	if got := values(AssetStatusInUse, false); got != "MANUTENCAO,BAIXADO" {
		t.Errorf("em uso: %s", got)
	}
	if got := values(AssetStatusRetired, false); got != "" {
		t.Errorf("baixado sem permissão: %s", got)
	}
	if got := values(AssetStatusRetired, true); got != "EM_USO" {
		t.Errorf("baixado com permissão: %s", got)
	}
}
