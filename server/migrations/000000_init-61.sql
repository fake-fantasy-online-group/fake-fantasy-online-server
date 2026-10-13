-- Cube catalogue: definitions are configuration; ownership/RNG/transactions remain Go.
-- Existing recipe IDs, prices, yields and item instances are unchanged.
CREATE TABLE IF NOT EXISTS game_cube_recipes (
 id INTEGER PRIMARY KEY CHECK (id > 0),
 name TEXT NOT NULL CHECK (btrim(name) <> ''),
 operation TEXT NOT NULL CHECK (operation IN ('salvage.normal','salvage.unique','salvage.rune','socket.open','socket.extract','socket.reorder','socket.array','unique.awaken','unique.reforge','unique.temper','rune.inscribe','rune.convert')),
 description TEXT NOT NULL,
 target_required BOOLEAN NOT NULL,
 money BIGINT NOT NULL CHECK (money >= 0),
 source_count INTEGER NOT NULL CHECK (source_count BETWEEN 0 AND 5),
 quality_step_bp INTEGER NOT NULL CHECK (quality_step_bp BETWEEN 0 AND 4000),
 material_count INTEGER NOT NULL CHECK (material_count >= 0),
 output_count INTEGER NOT NULL CHECK (output_count >= 0)
);
CREATE TABLE IF NOT EXISTS game_cube_materials (
 recipe_id INTEGER NOT NULL REFERENCES game_cube_recipes(id) ON DELETE CASCADE,
 seq INTEGER NOT NULL CHECK (seq > 0),
 item_id INTEGER NOT NULL REFERENCES game_items(id),
 count INTEGER NOT NULL CHECK (count > 0),
 PRIMARY KEY (recipe_id, seq), UNIQUE (recipe_id, item_id)
);
CREATE TABLE IF NOT EXISTS game_cube_outputs (
 recipe_id INTEGER NOT NULL REFERENCES game_cube_recipes(id) ON DELETE CASCADE,
 seq INTEGER NOT NULL CHECK (seq > 0),
 item_id INTEGER NOT NULL REFERENCES game_items(id),
 count INTEGER NOT NULL CHECK (count > 0),
 level_divisor INTEGER NOT NULL CHECK (level_divisor >= 0),
 level_cap INTEGER NOT NULL CHECK (level_cap >= 0),
 multiplier INTEGER NOT NULL CHECK (multiplier > 0),
 PRIMARY KEY (recipe_id, seq), UNIQUE (recipe_id, item_id)
);
DO $migration$
DECLARE new_ids INTEGER[];
 seed JSONB := $cube_rules$
[
  {
    "ID": 1001,
    "Name": "解器归元",
    "Operation": "salvage.normal",
    "Description": "消耗普通可染色装备，获得星髓；孔内卡原样返还，换形与装备灵随装备消失。",
    "TargetRequired": true,
    "Money": 0,
    "Materials": [],
    "Outputs": [
      {
        "ItemID": 1900028201,
        "Count": 8,
        "LevelDivisor": 5,
        "LevelCap": 100,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 1002,
    "Name": "仙遗化韵",
    "Operation": "salvage.unique",
    "Description": "消耗仙遗，获得星髓与八仙余韵；孔内卡原样返还，换形与装备灵随装备消失。",
    "TargetRequired": true,
    "Money": 0,
    "Materials": [],
    "Outputs": [
      {
        "ItemID": 1900028203,
        "Count": 4,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      },
      {
        "ItemID": 1900028201,
        "Count": 8,
        "LevelDivisor": 5,
        "LevelCap": 100,
        "Multiplier": 3
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 1003,
    "Name": "星篆归尘",
    "Operation": "salvage.rune",
    "Description": "消耗所选一枚背包星篆，获得4星髓。",
    "TargetRequired": false,
    "Money": 0,
    "Materials": [],
    "Outputs": [
      {
        "ItemID": 1900028201,
        "Count": 4,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 1006,
    "Name": "开窍引星",
    "Operation": "socket.open",
    "Description": "仅限无孔装备，随机开出1至真实容量的孔数，各孔数等概率；已有孔不可重开。",
    "TargetRequired": true,
    "Money": 5000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 4
      }
    ],
    "Outputs": [],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 1007,
    "Name": "归星还篆",
    "Operation": "socket.extract",
    "Description": "原样取回全部孔内卡，保留空孔；背包不足则整笔不执行。",
    "TargetRequired": true,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 4
      },
      {
        "ItemID": 1900028202,
        "Count": 2
      }
    ],
    "Outputs": [],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 1008,
    "Name": "移星换斗",
    "Operation": "socket.reorder",
    "Description": "按所选顺序重排已镶满的全部星篆，保留每枚身份与属性。",
    "TargetRequired": true,
    "Money": 5000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 2
      },
      {
        "ItemID": 1900028202,
        "Count": 1
      }
    ],
    "Outputs": [],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 1009,
    "Name": "布宿成阵",
    "Operation": "socket.array",
    "Description": "将所选星篆依次放入全部空孔，须完全匹配一项宿阵的孔数、顺序与部位。",
    "TargetRequired": true,
    "Money": 15000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 1010,
    "Name": "淬器成真",
    "Operation": "unique.awaken",
    "Description": "唤醒此基底唯一对应的仙遗；主体属性固定，少数词条浮动；原有加工、孔内卡与换形保留。",
    "TargetRequired": true,
    "Money": 100000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 120
      },
      {
        "ItemID": 1900028202,
        "Count": 30
      }
    ],
    "Outputs": [],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 1011,
    "Name": "返本重铸",
    "Operation": "unique.reforge",
    "Description": "仅重铸明确浮动的词条，保证与当前可见数值不同，可能降低；固定属性、原有养成与仙遗身份保留。",
    "TargetRequired": true,
    "Money": 100000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 40
      },
      {
        "ItemID": 1900028202,
        "Count": 25
      },
      {
        "ItemID": 1900028203,
        "Count": 4
      }
    ],
    "Outputs": [],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 1012,
    "Name": "炼真守一",
    "Operation": "unique.temper",
    "Description": "提高少数浮动词条的共享品质，确保可见数值提升；已满值不可使用，固定属性与原有养成不变。",
    "TargetRequired": true,
    "Money": 200000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 60
      },
      {
        "ItemID": 1900028202,
        "Count": 40
      },
      {
        "ItemID": 1900028203,
        "Count": 6
      }
    ],
    "Outputs": [],
    "SourceCount": 0,
    "QualityStepBP": 500
  },
  {
    "ID": 11001,
    "Name": "凝髓成篆·星篆·角木蛟",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028001,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12001,
    "Name": "同象易宿·星篆·角木蛟",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028001,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11002,
    "Name": "凝髓成篆·星篆·亢金龙",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028002,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12002,
    "Name": "同象易宿·星篆·亢金龙",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028002,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11003,
    "Name": "凝髓成篆·星篆·氐土貉",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028003,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12003,
    "Name": "同象易宿·星篆·氐土貉",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028003,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11004,
    "Name": "凝髓成篆·星篆·房日兔",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028004,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12004,
    "Name": "同象易宿·星篆·房日兔",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028004,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11005,
    "Name": "凝髓成篆·星篆·心月狐",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028005,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12005,
    "Name": "同象易宿·星篆·心月狐",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028005,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11006,
    "Name": "凝髓成篆·星篆·尾火虎",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028006,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12006,
    "Name": "同象易宿·星篆·尾火虎",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028006,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11007,
    "Name": "凝髓成篆·星篆·箕水豹",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028007,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12007,
    "Name": "同象易宿·星篆·箕水豹",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028007,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11008,
    "Name": "凝髓成篆·星篆·斗木獬",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028008,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12008,
    "Name": "同象易宿·星篆·斗木獬",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028008,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11009,
    "Name": "凝髓成篆·星篆·牛金牛",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028009,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12009,
    "Name": "同象易宿·星篆·牛金牛",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028009,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11010,
    "Name": "凝髓成篆·星篆·女土蝠",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028010,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12010,
    "Name": "同象易宿·星篆·女土蝠",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028010,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11011,
    "Name": "凝髓成篆·星篆·虚日鼠",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028011,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12011,
    "Name": "同象易宿·星篆·虚日鼠",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028011,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11012,
    "Name": "凝髓成篆·星篆·危月燕",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028012,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12012,
    "Name": "同象易宿·星篆·危月燕",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028012,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11013,
    "Name": "凝髓成篆·星篆·室火猪",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028013,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12013,
    "Name": "同象易宿·星篆·室火猪",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028013,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11014,
    "Name": "凝髓成篆·星篆·壁水貐",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028014,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12014,
    "Name": "同象易宿·星篆·壁水貐",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028014,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11015,
    "Name": "凝髓成篆·星篆·奎木狼",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028015,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12015,
    "Name": "同象易宿·星篆·奎木狼",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028015,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11016,
    "Name": "凝髓成篆·星篆·娄金狗",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028016,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12016,
    "Name": "同象易宿·星篆·娄金狗",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028016,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11017,
    "Name": "凝髓成篆·星篆·胃土雉",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028017,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12017,
    "Name": "同象易宿·星篆·胃土雉",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028017,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11018,
    "Name": "凝髓成篆·星篆·昴日鸡",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028018,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12018,
    "Name": "同象易宿·星篆·昴日鸡",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028018,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11019,
    "Name": "凝髓成篆·星篆·毕月乌",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028019,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12019,
    "Name": "同象易宿·星篆·毕月乌",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028019,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11020,
    "Name": "凝髓成篆·星篆·觜火猴",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028020,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12020,
    "Name": "同象易宿·星篆·觜火猴",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028020,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11021,
    "Name": "凝髓成篆·星篆·参水猿",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028021,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12021,
    "Name": "同象易宿·星篆·参水猿",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028021,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11022,
    "Name": "凝髓成篆·星篆·井木犴",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028022,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12022,
    "Name": "同象易宿·星篆·井木犴",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028022,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11023,
    "Name": "凝髓成篆·星篆·鬼金羊",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028023,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12023,
    "Name": "同象易宿·星篆·鬼金羊",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028023,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11024,
    "Name": "凝髓成篆·星篆·柳土獐",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028024,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12024,
    "Name": "同象易宿·星篆·柳土獐",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028024,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11025,
    "Name": "凝髓成篆·星篆·星日马",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028025,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12025,
    "Name": "同象易宿·星篆·星日马",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028025,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11026,
    "Name": "凝髓成篆·星篆·张月鹿",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028026,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12026,
    "Name": "同象易宿·星篆·张月鹿",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028026,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11027,
    "Name": "凝髓成篆·星篆·翼火蛇",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028027,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12027,
    "Name": "同象易宿·星篆·翼火蛇",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028027,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  },
  {
    "ID": 11028,
    "Name": "凝髓成篆·星篆·轸水蚓",
    "Operation": "rune.inscribe",
    "Description": "凝成一枚指定星篆。",
    "TargetRequired": false,
    "Money": 30000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 30
      },
      {
        "ItemID": 1900028202,
        "Count": 10
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028028,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 0,
    "QualityStepBP": 0
  },
  {
    "ID": 12028,
    "Name": "同象易宿·星篆·轸水蚓",
    "Operation": "rune.convert",
    "Description": "消耗三枚相同星篆，换得指定的同象异宿星篆。",
    "TargetRequired": false,
    "Money": 10000,
    "Materials": [
      {
        "ItemID": 1900028201,
        "Count": 6
      },
      {
        "ItemID": 1900028202,
        "Count": 3
      }
    ],
    "Outputs": [
      {
        "ItemID": 1900028028,
        "Count": 1,
        "LevelDivisor": 0,
        "LevelCap": 0,
        "Multiplier": 1
      }
    ],
    "SourceCount": 3,
    "QualityStepBP": 0
  }
]
$cube_rules$::jsonb;
BEGIN
 WITH inserted AS (INSERT INTO game_cube_recipes (id,name,operation,description,target_required,money,source_count,quality_step_bp,material_count,output_count)
 SELECT (r->>'ID')::int,r->>'Name',r->>'Operation',r->>'Description',(r->>'TargetRequired')::boolean,(r->>'Money')::bigint,(r->>'SourceCount')::int,(r->>'QualityStepBP')::int,jsonb_array_length(r->'Materials'),jsonb_array_length(r->'Outputs')
 FROM jsonb_array_elements(seed) r
 ON CONFLICT (id) DO NOTHING RETURNING id)
 SELECT coalesce(array_agg(id),'{}'::integer[]) INTO new_ids FROM inserted;
 INSERT INTO game_cube_materials (recipe_id,seq,item_id,count)
 SELECT (r->>'ID')::int,m.ordinality::int,(m.value->>'ItemID')::int,(m.value->>'Count')::int
 FROM jsonb_array_elements(seed) r CROSS JOIN LATERAL jsonb_array_elements(r->'Materials') WITH ORDINALITY m
 WHERE (r->>'ID')::int = ANY(new_ids)
 ON CONFLICT (recipe_id,seq) DO NOTHING;
 INSERT INTO game_cube_outputs (recipe_id,seq,item_id,count,level_divisor,level_cap,multiplier)
 SELECT (r->>'ID')::int,o.ordinality::int,(o.value->>'ItemID')::int,(o.value->>'Count')::int,(o.value->>'LevelDivisor')::int,(o.value->>'LevelCap')::int,(o.value->>'Multiplier')::int
 FROM jsonb_array_elements(seed) r CROSS JOIN LATERAL jsonb_array_elements(r->'Outputs') WITH ORDINALITY o
 WHERE (r->>'ID')::int = ANY(new_ids)
 ON CONFLICT (recipe_id,seq) DO NOTHING;
END
$migration$;
