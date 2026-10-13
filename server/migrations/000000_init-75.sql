-- 周天鼎：复用左慈身后原生商会房间和已有火焰鼎外观。
-- 原场景 map/qw0007.gsm 的 DH_FY_1501 门位于 (9155,5907)，
-- proc 5 原触发面中心 (9250,6060)，直达 map 506 / qw0506 商会。
-- qw0506 的原 cover、家具、出口均原样复用；不创建地图、贴图或业务表。
-- avatar/npc/npc90301.gsn 已核对原始像素，为金绿三足火焰鼎；902 是另一种机械炉。
-- 新放置点 (620,430) 位于室内木台可行走格，原 msk 已验证其与入点/出口连通。
-- N903、N611FY 及所有既有 NPC 的服务含义保持不变。

DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM map_defs WHERE id=7 AND file='qw0007') OR
       NOT EXISTS (SELECT FROM map_defs WHERE id=506 AND file='qw0506') THEN
        RAISE EXCEPTION 'Zhoutian Ding requires original Longcheng and chamber maps';
    END IF;
    IF NOT EXISTS (SELECT FROM map_npcs WHERE map_file='qw0007' AND npc_id=73
                   AND sprite=611 AND script='N611FY') THEN
        RAISE EXCEPTION 'Zhoutian Ding requires the original Zuoci placement';
    END IF;
    IF NOT EXISTS (SELECT FROM map_teleports WHERE map_file='qw0007' AND proc_id=5
                   AND map_to=506 AND to_x=498 AND to_y=541) OR
       NOT EXISTS (SELECT FROM map_teleports WHERE map_file='qw0506' AND proc_id=1
                   AND map_to=7 AND (to_x,to_y) IN ((8898,6707),(9330,6099))) THEN
        RAISE EXCEPTION 'Zhoutian Ding requires the original reversible chamber doorway';
    END IF;
    IF EXISTS (SELECT FROM map_npcs WHERE map_file='qw0506' AND npc_id=1
               AND script <> 'Nzhoutianding') THEN
        RAISE EXCEPTION 'Zhoutian Ding chamber NPC slot is already occupied';
    END IF;
END $$;

-- 原 init-0 室内出口的 poly UPDATE 以新版龙城落点为条件，旧数据落点未平移，
-- 因此未命中并遗留 NULL。只闭合这对门：原龙城坐标修正 (+432,-608)，
-- polygon 原值来自 init-0 qw0506 proc 1；不猜测、不扩张门范围。
-- 出口落点 (9330,6099) 在龙城入口触发面之外，且原 msk 验证可行走。
UPDATE map_teleports
SET back_x=9330, back_y=6099,
    poly=ARRAY[9250,5980,9385,6060,9250,6140,9115,6060]
WHERE map_file='qw0007' AND proc_id=5 AND map_to=506;

UPDATE map_teleports
SET to_x=9330, to_y=6099,
    poly=ARRAY[583,580,630,620,675,591,642,550]
WHERE map_file='qw0506' AND proc_id=1 AND map_to=7;

-- 数据库存放专用交互脚本和位置；显示仍走原生 map_npcs / NPC sprite 通道。
-- replay 不覆盖管理员对这个新 NPC 的后续坐标、方向或名称配置。
INSERT INTO map_npcs (map_file,npc_id,sprite,name,script,x,y,dir,sell_list,trans_list,portrait,hide)
VALUES ('qw0506',1,903,'周天鼎','Nzhoutianding',620,430,67,0,0,'',FALSE)
ON CONFLICT (map_file,npc_id) DO NOTHING;
