package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Load the same persisted definitions used by administrative tools. Only publish
// the immutable rule snapshots after all required tables have been validated.
func loadEndgameData(ctx context.Context, q data.Querier, w *worldData, log *slog.Logger) {
	catalog, err := data.LoadEndgameCatalog(ctx, q, w.items, w.skills.ProgressionCatalog())
	if err != nil {
		w.endgameDataErr = fmt.Errorf("load endgame catalog: %w", err)
		log.Error("演劫目录加载失败", "err", w.endgameDataErr)
		return
	}
	rules, err := data.LoadCubeRules(ctx, q, w.items)
	if err != nil {
		w.endgameDataErr = fmt.Errorf("load cube rules: %w", err)
		log.Error("魔盒配方加载失败", "err", w.endgameDataErr)
		return
	}
	if err := domain.ValidateCubeSnapshotMaterials(w.items, catalog, rules); err != nil {
		w.endgameDataErr = fmt.Errorf("validate cube snapshot materials: %w", err)
		return
	}
	if err := domain.ValidateEquipmentRuntimeV5(w.skills, catalog); err != nil {
		w.endgameDataErr = fmt.Errorf("validate equipment skill references: %w", err)
		log.Error("装备技能规则校验失败", "err", w.endgameDataErr)
		return
	}
	loot, err := data.LoadLootRules(ctx, q, w.items)
	if err != nil {
		w.endgameDataErr = fmt.Errorf("load supplemental loot rules: %w", err)
		return
	}
	rifts, err := data.LoadRiftRules(ctx, q, loot, w.items)
	if err != nil {
		w.endgameDataErr = fmt.Errorf("load rift rules: %w", err)
		return
	}
	domain.AttachEndgameCatalog(w.items, catalog)
	w.endgameCatalog, w.cubeRules = catalog, rules
	w.lootRules, w.riftRules = loot, rifts
	log.Info("演劫数据库目录与魔盒配方就绪")
}
