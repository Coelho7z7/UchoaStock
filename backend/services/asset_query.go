package services

import (
	"database/sql"
	"errors"
	"strings"

	database "uchoastock/backend/database"
	"uchoastock/backend/models"
)

// Situações do bem, como ficam gravadas no banco.
const (
	AssetStatusInUse       = "EM_USO"
	AssetStatusMaintenance = "MANUTENCAO"
	AssetStatusRetired     = "BAIXADO"
)

// AssetStatusAll é o valor do filtro que traz também os bens baixados. O
// filtro vazio (o padrão) esconde os baixados: eles não estão mais em
// obra nenhuma, só ficam para o histórico.
const AssetStatusAll = "TODAS"

// Ações do histórico do bem, como ficam gravadas no banco.
const (
	AssetEventCreated  = "CADASTRO"
	AssetEventTransfer = "TRANSFERENCIA"
	AssetEventStatus   = "SITUACAO"
	AssetEventEdit     = "EDICAO"
)

// AssetStatusLabels liga cada situação ao texto da tela, na ordem do
// filtro e do formulário.
var AssetStatusLabels = []struct{ Value, Label string }{
	{AssetStatusInUse, "Em uso"},
	{AssetStatusMaintenance, "Em manutenção"},
	{AssetStatusRetired, "Baixado"},
}

var assetEventLabels = map[string]string{
	AssetEventCreated:  "Cadastro",
	AssetEventTransfer: "Transferência",
	AssetEventStatus:   "Mudança de situação",
	AssetEventEdit:     "Dados alterados",
}

// assetStatusLabel devolve o texto da situação, ou "" se ela não existe.
func assetStatusLabel(status string) string {
	for _, s := range AssetStatusLabels {
		if s.Value == status {
			return s.Label
		}
	}
	return ""
}

func isValidAssetStatus(status string) bool {
	return assetStatusLabel(status) != ""
}

// assetSelect são as colunas lidas em toda consulta de bem, na ordem em
// que scanAsset as espera.
const assetSelect = `
	SELECT a.id, a.numero, a.nome, a.descricao, a.obra_id, o.nome, a.situacao,
		strftime('%d/%m/%Y', a.criado_em, 'localtime')
	FROM patrimonios a
	JOIN obras o ON o.id = a.obra_id
`

func scanAsset(row rowScanner) (models.Asset, error) {
	var asset models.Asset
	if err := row.Scan(
		&asset.ID,
		&asset.Number,
		&asset.Name,
		&asset.Description,
		&asset.SiteID,
		&asset.SiteName,
		&asset.Status,
		&asset.CreatedAt,
	); err != nil {
		return asset, err
	}
	asset.FormattedStatus = assetStatusLabel(asset.Status)
	return asset, nil
}

// GetAssets lista os bens da obra siteID (0 = todas as obras). search
// procura no número, no nome e na descrição. status vazio traz os bens em
// uso e em manutenção; AssetStatusAll traz todos, inclusive os baixados;
// uma situação válida traz só ela.
func GetAssets(siteID int, search, status string) ([]models.Asset, error) {
	query := assetSelect + ` WHERE 1 = 1`
	var args []any

	if siteID > 0 {
		query += ` AND a.obra_id = ?`
		args = append(args, siteID)
	}

	if search = strings.TrimSpace(search); search != "" {
		query += ` AND (a.numero LIKE ? OR a.nome LIKE ? OR a.descricao LIKE ?)`
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern, pattern)
	}

	switch {
	case status == AssetStatusAll:
	case isValidAssetStatus(status):
		query += ` AND a.situacao = ?`
		args = append(args, status)
	default:
		query += ` AND a.situacao <> ?`
		args = append(args, AssetStatusRetired)
	}

	query += ` ORDER BY a.nome COLLATE NOCASE, a.numero COLLATE NOCASE`

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []models.Asset
	for rows.Next() {
		asset, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	return assets, rows.Err()
}

// GetAssetByID busca um bem pelo ID. Devolve ErrAssetNotFound se ele não
// existir.
func GetAssetByID(id int) (*models.Asset, error) {
	asset, err := scanAsset(database.DB.QueryRow(assetSelect+` WHERE a.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAssetNotFound
	}
	if err != nil {
		return nil, err
	}
	return &asset, nil
}

// GetAssetEvents devolve o histórico do bem, do mais recente para o mais
// antigo. LEFT JOIN nas obras porque origem e destino são opcionais.
func GetAssetEvents(assetID int) ([]models.AssetEvent, error) {
	rows, err := database.DB.Query(`
		SELECT e.acao, u.nome, COALESCE(origem.nome, ''), COALESCE(destino.nome, ''),
			e.situacao, e.observacao, strftime('%d/%m/%Y %H:%M', e.criado_em, 'localtime')
		FROM patrimonio_movimentacoes e
		JOIN usuarios u ON u.id = e.usuario_id
		LEFT JOIN obras origem ON origem.id = e.obra_origem_id
		LEFT JOIN obras destino ON destino.id = e.obra_destino_id
		WHERE e.patrimonio_id = ?
		ORDER BY e.id DESC
	`, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.AssetEvent
	for rows.Next() {
		var event models.AssetEvent
		if err := rows.Scan(
			&event.Action,
			&event.UserName,
			&event.FromSiteName,
			&event.ToSiteName,
			&event.Status,
			&event.Note,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		event.FormattedAction = assetEventLabels[event.Action]
		event.FormattedStatus = assetStatusLabel(event.Status)
		events = append(events, event)
	}
	return events, rows.Err()
}
