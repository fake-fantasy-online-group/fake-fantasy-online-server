package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Each catalog uses the same PostgreSQL definitions as the game loaders. NPC IDs
// are map-local; the map ID is always included in their stable key and actions.
const npcCatalogSQL = `SELECT n.npc_id AS id, n.name, 0 AS level, n.sell_list AS type_code,
 d.id AS map_id, ''::text AS profession,
 CASE WHEN n.hide THEN 'hidden' ELSE 'active' END AS status,
 jsonb_build_object('kind','npc','id',n.npc_id,'key',d.id||':'||n.npc_id,
 'name',n.name,'mapId',d.id,'mapName',d.name,'x',n.x,'y',n.y,
 'shopId',n.sell_list,'transportId',n.trans_list,'hidden',n.hide,
 'taskCount',COALESCE(task_counts.amount,0),
 'description',n.script) AS data
 FROM map_npcs n JOIN map_defs d ON d.file=n.map_file
 LEFT JOIN (
   SELECT COALESCE(f.publisher,t.npc) AS publisher, count(*) AS amount
   FROM game_tasks t
   LEFT JOIN (SELECT DISTINCT ON (task_id) task_id,publisher FROM game_task_flows ORDER BY task_id,layer DESC) f ON f.task_id=t.id
   GROUP BY COALESCE(f.publisher,t.npc)
 ) task_counts ON task_counts.publisher=n.name`

const taskCatalogSQL = `SELECT t.id, t.name, t.level_min AS level, 0 AS type_code,
 0 AS map_id, ''::text AS profession,
 CASE WHEN disabled.task_id IS NOT NULL THEN 'disabled' ELSE 'active' END AS status,
 jsonb_build_object('kind','task','id',t.id,'key',t.id::text,'name',t.name,
 'level',t.level_min,'maxLevel',t.level_max,'description',t.descr,
 'npc',COALESCE(f.publisher,t.npc),'mapName',t.map,'reward',t.reward,'repeatable',t.repeatable,
 'disabled',disabled.task_id IS NOT NULL,'disabledReason',COALESCE(disabled.disabled_reason,''),
 'exp',COALESCE(r.exp,0),'copper',COALESCE(r.copper,0)) AS data
 FROM game_tasks t
 LEFT JOIN LATERAL (SELECT publisher FROM game_task_flows WHERE task_id=t.id ORDER BY layer DESC LIMIT 1) f ON true
 LEFT JOIN game_task_disabled disabled ON disabled.task_id=t.id
 LEFT JOIN game_task_rewards r ON r.task_id=t.id`

const skillCatalogSQL = `SELECT s.skill_id AS id, s.name, s.level_need AS level,
 s.skill_type AS type_code, 0 AS map_id, s.prof::text AS profession,
 CASE WHEN s.prof BETWEEN 1 AND 5 OR s.skill_id IN (13601,13602,11001,11002,11003,11004,11006,11007,11008,11009,11010)
 OR (s.prof=0 AND s.binding_arm>0 AND EXISTS (SELECT 1 FROM gamedata.ov_arm a WHERE a.index=s.binding_arm AND a.binding_skill=s.skill_id))
 THEN 'active' ELSE 'reference' END AS status,
 jsonb_build_object('kind','skill','id',s.skill_id,'key',s.skill_id||':'||s.skill_level,
 'name',s.name,'level',s.level_need,'skillLevel',s.skill_level,'maxLevel',s.max_level,
 'profession',s.prof,'typeCode',s.skill_type,'description',s.desc_,
 'bookId',s.book_id,'bindingArm',s.binding_arm,'learnMoney',s.learn_money,
 'cooldown',s.sep::numeric/10,'castTime',s.prepare,'distance',s.dist,
 'dependencies',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',v.id,'level',v.level))
 FROM (VALUES (s.depend1_id,s.depend1_qty),(s.depend2_id,s.depend2_qty),
 (s.depend3_id,s.depend3_qty),(s.depend4_id,s.depend4_qty)) v(id,level) WHERE v.id>0),'[]'::jsonb)) AS data
 FROM (SELECT DISTINCT ON (skill_id,skill_level) * FROM gamedata.ov_skilldesc
 WHERE skill_id>0 AND skill_level>0 AND btrim(name)<>'' ORDER BY skill_id,skill_level,row_no) s`

func (s *catalogStore) catalogReady(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSON(w, 405, map[string]string{"error": "仅支持 GET"})
		return false
	}
	if s == nil || s.pool == nil {
		writeJSON(w, 503, map[string]string{"error": "数据库暂不可用"})
		return false
	}
	return true
}

func (s *catalogStore) entriesHandler(w http.ResponseWriter, r *http.Request) {
	if !s.catalogReady(w, r) {
		return
	}
	q := r.URL.Query()
	base, ok := map[string]string{"npc": npcCatalogSQL, "task": taskCatalogSQL, "skill": skillCatalogSQL}[q.Get("kind")]
	if !ok {
		writeJSON(w, 400, map[string]string{"error": "kind 只支持 npc、task、skill"})
		return
	}
	where := []string{"true"}
	args := []any{}
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	fail := func(message string) { writeJSON(w, 400, map[string]string{"error": message}) }
	if query := strings.TrimSpace(q.Get("q")); query != "" {
		if len([]rune(query)) > 80 {
			fail("搜索词最多 80 个字符")
			return
		}
		if id, err := strconv.ParseInt(query, 10, 64); err == nil && id > 0 {
			where = append(where, "id = "+add(id))
		} else {
			p := add("%" + query + "%")
			where = append(where, "(name ILIKE "+p+" OR data->>'description' ILIKE "+p+" OR data->>'npc' ILIKE "+p+" OR data->>'mapName' ILIKE "+p+")")
		}
	}
	for _, f := range []struct{ key, column, op string }{{"minLevel", "level", ">="}, {"maxLevel", "level", "<="}, {"mapId", "map_id", "="}, {"category", "type_code", "="}, {"skillLevel", "(data->>'skillLevel')::int", "="}} {
		if raw := q.Get(f.key); raw != "" {
			n, err := optionalNonNegativeInt(raw)
			if err != nil {
				fail("筛选值必须是非负整数")
				return
			}
			where = append(where, f.column+" "+f.op+" "+add(n))
		}
	}
	if min, max := q.Get("minLevel"), q.Get("maxLevel"); min != "" && max != "" {
		lo, _ := strconv.ParseInt(min, 10, 64)
		hi, _ := strconv.ParseInt(max, 10, 64)
		if lo > hi {
			fail("最低等级不能高于最高等级")
			return
		}
	}
	for _, f := range []struct{ key, column string }{{"profession", "profession"}, {"status", "status"}, {"npc", "data->>'npc'"}} {
		if raw := q.Get(f.key); raw != "" {
			if len(raw) > 100 {
				fail("筛选值过长")
				return
			}
			where = append(where, f.column+" = "+add(raw))
		}
	}
	if q.Get("service") == "shop" {
		where = append(where, "(data->>'shopId')::int>0")
	}
	if q.Get("service") == "transport" {
		where = append(where, "(data->>'transportId')::int>0")
	}
	if q.Get("service") == "task" {
		where = append(where, "(data->>'taskCount')::int>0")
	}
	if q.Get("repeatable") == "1" {
		where = append(where, "(data->>'repeatable')::boolean")
	}
	page, size := 1, 24
	for _, f := range []struct {
		key string
		dst *int
		max int
	}{{"page", &page, 1000000}, {"pageSize", &size, 60}} {
		if raw := q.Get(f.key); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil || n < 1 || n > f.max {
				fail("分页参数超出范围")
				return
			}
			*f.dst = n
		}
	}
	sort := q.Get("sort")
	if sort == "" {
		sort = "id-asc"
	}
	order, ok := map[string]string{"id-asc": "id ASC", "id-desc": "id DESC", "name-asc": "name ASC, id ASC", "level-asc": "level ASC, id ASC", "level-desc": "level DESC, id ASC"}[sort]
	if !ok {
		fail("不支持的排序方式")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	cte := "WITH entries AS (" + base + ") "
	clause := " FROM entries WHERE " + strings.Join(where, " AND ")
	var total int64
	err := s.pool.QueryRow(ctx, cte+"SELECT count(*)"+clause, args...).Scan(&total)
	var rows []json.RawMessage
	if err == nil {
		limit, offset := add(size), add((page-1)*size)
		rows, err = s.queryJSON(ctx, cte+"SELECT data || jsonb_build_object('status',status)"+clause+" ORDER BY "+order+", data->>'key' LIMIT "+limit+" OFFSET "+offset, args...)
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": rows, "total": total, "page": page, "pageSize": size})
}

func (s *catalogStore) queryJSON(ctx context.Context, query string, args ...any) ([]json.RawMessage, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]json.RawMessage, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		result = append(result, json.RawMessage(raw))
	}
	return result, rows.Err()
}

func catalogError(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, 404, map[string]string{"error": "资料已不存在，请刷新列表"})
		return
	}
	log.Printf("查询 GM 工作台失败: %v", err)
	writeJSON(w, 500, map[string]string{"error": "查询失败，请稍后重试"})
}

func (s *catalogStore) relationsHandler(w http.ResponseWriter, r *http.Request) {
	if !s.catalogReady(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 32)
	if err != nil || id <= 0 {
		writeJSON(w, 400, map[string]string{"error": "无效的编号"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result := map[string]any{}
	var query string
	switch r.URL.Query().Get("kind") {
	case "npc":
		mapID, e := strconv.ParseInt(r.URL.Query().Get("mapId"), 10, 32)
		if e != nil || mapID <= 0 {
			writeJSON(w, 400, map[string]string{"error": "NPC 必须带地图编号"})
			return
		}
		var name string
		var shop int64
		err = s.pool.QueryRow(ctx, `SELECT n.name,n.sell_list FROM map_npcs n JOIN map_defs d ON d.file=n.map_file WHERE n.npc_id=$1 AND d.id=$2`, id, mapID).Scan(&name, &shop)
		if err == nil {
			result["tasks"], err = s.queryJSON(ctx, `SELECT data FROM (`+taskCatalogSQL+`) t WHERE data->>'npc'=$1 ORDER BY level,id`, name)
		}
		if err == nil {
			result["items"], err = s.queryJSON(ctx, `SELECT jsonb_build_object('id',i.item_id,'name',COALESCE(a.name,b.name,'物品 '||i.item_id),'qty',1)
   FROM game_shops s JOIN game_shop_items i ON i.shop_id=COALESCE(s.stock_source_shop_id,s.shop_id)
   LEFT JOIN game_items a ON a.id=i.item_id LEFT JOIN game_equipment b ON b.id=i.item_id WHERE s.shop_id=$1 ORDER BY i.slot`, shop)
		}
	case "task":
		result["npcs"], err = s.queryJSON(ctx, `SELECT data FROM (`+npcCatalogSQL+`) n WHERE name=(SELECT COALESCE((SELECT publisher FROM game_task_flows WHERE task_id=$1 ORDER BY layer DESC LIMIT 1),npc) FROM game_tasks WHERE id=$1) ORDER BY map_id,id`, id)
		if err == nil {
			result["prerequisites"], err = s.queryJSON(ctx, `SELECT jsonb_build_object('id',t.id,'name',t.name) FROM game_task_prerequisites p JOIN game_tasks t ON t.id=p.prerequisite_task_id WHERE p.task_id=$1 ORDER BY p.seq`, id)
		}
		if err == nil {
			result["rewards"], err = s.queryJSON(ctx, `SELECT jsonb_build_object('id',r.item_id,'name',COALESCE(i.name,e.name,'物品 '||r.item_id),'qty',r.qty) FROM game_task_reward_items r LEFT JOIN game_items i ON i.id=r.item_id LEFT JOIN game_equipment e ON e.id=r.item_id WHERE task_id=$1 ORDER BY seq`, id)
		}
		if err == nil {
			result["steps"], err = s.queryJSON(ctx, `SELECT jsonb_build_object('number',s.step_no,'npc',s.target_npc,'kind',s.kind,'text',s.text,'say',s.say,
   'kills',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',k.monster_id,'name',k.monster_name,'qty',k.qty) ORDER BY k.ordinal) FROM game_task_flow_kills k WHERE k.layer=s.layer AND k.task_id=s.task_id AND k.step_no=s.step_no),'[]'::jsonb),
   'items',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',i.item_id,'name',i.item_name,'qty',i.qty,'phase',i.phase) ORDER BY i.phase,i.ordinal) FROM game_task_flow_step_items i WHERE i.layer=s.layer AND i.task_id=s.task_id AND i.step_no=s.step_no),'[]'::jsonb))
   FROM game_task_flow_steps s WHERE task_id=$1 AND layer=(SELECT layer FROM game_task_flows WHERE task_id=$1 ORDER BY layer DESC LIMIT 1) ORDER BY step_no`, id)
		}
	case "item", "equipment":
		query = `SELECT data FROM (` + npcCatalogSQL + `) n WHERE (data->>'shopId')::int IN (SELECT s.shop_id FROM game_shops s JOIN game_shop_items i ON i.shop_id=COALESCE(s.stock_source_shop_id,s.shop_id) WHERE i.item_id=$1) ORDER BY map_id,id`
		result["npcs"], err = s.queryJSON(ctx, query, id)
		if err == nil {
			result["tasks"], err = s.queryJSON(ctx, `SELECT data FROM (`+taskCatalogSQL+`) t WHERE id IN (SELECT task_id FROM game_task_reward_items WHERE item_id=$1) ORDER BY level,id`, id)
		}
		if err == nil {
			result["drops"], err = s.queryJSON(ctx, `SELECT jsonb_build_object('name',monster_name,'id',COALESCE(monster_id,0)) FROM game_drops WHERE item_id=$1 ORDER BY monster_name`, id)
		}
	default:
		err = errors.New("unsupported kind")
		writeJSON(w, 400, map[string]string{"error": "此类型没有关联查询"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
