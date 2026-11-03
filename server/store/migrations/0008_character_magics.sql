-- STAGE-48：人物的精灵栏位（最多 5 个，协议 10.5 的 magic 0–4）。
-- 数组按栏位存精灵编号（magic.tsv 的 ID），NULL 表示空栏位。
ALTER TABLE characters ADD COLUMN magics integer[5] NOT NULL DEFAULT ARRAY[-1,-1,-1,-1,-1];
