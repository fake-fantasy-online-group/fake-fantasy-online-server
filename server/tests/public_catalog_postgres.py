#!/usr/bin/env python3
"""Validate the public export SQL in a new, socket-free synthetic PostgreSQL DB.

No existing database or game resources are read. Binaries must already be
installed from a trusted source; this script installs/downloads nothing.
"""

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]


def run(command, **kwargs):
    result = subprocess.run(command, text=True, capture_output=True, check=True, **kwargs)
    return result.stdout + result.stderr


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pg-bin", required=True, type=Path, help="trusted PostgreSQL bin directory")
    args = parser.parse_args()
    query = (ROOT / "internal/publiccatalog/query.go").read_text().split("const publicCatalogSQL = `", 1)[1].rsplit("`", 1)[0]
    fixture = (ROOT / "internal/publiccatalog/testdata/synthetic.sql").read_text()
    with tempfile.TemporaryDirectory(prefix="public-catalog-postgres-") as work:
        data = Path(work) / "data"
        environment = {**os.environ, "LC_ALL": "C", "TZ": "UTC"}
        run([str(args.pg_bin / "initdb"), "-D", str(data), "--auth=trust", "--no-locale", "--encoding=UTF8"], env=environment)

        def sql(statement, expect_error=None):
            output = run([str(args.pg_bin / "postgres"), "--single", "-D", str(data), "-j", "postgres"], input=statement, env=environment)
            errors = re.findall(r"(?:ERROR|FATAL):[^\n]+", output)
            if expect_error:
                assert len(errors) == 1 and expect_error in errors[0], output
            else:
                assert not errors, output
            return output

        sql(fixture)
        output = sql(
            "BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;\n"
            "SET LOCAL ROLE public_catalog_fixture_reader;\n"
            "SET LOCAL search_path = pg_catalog;\n"
            "SELECT current_setting('transaction_read_only') AS read_only, current_setting('transaction_isolation') AS isolation;\n"
            "PREPARE export_items(integer) AS SELECT row_to_json(exported) FROM (" + query + ") exported;\n"
            "EXECUTE export_items(4);\nROLLBACK;\n"
        )
        assert 'read_only = "on"' in output and 'isolation = "repeatable read"' in output, output
        records = [json.loads(raw) for raw in re.findall(r'row_to_json = "(.*)"\t\(typeid', output)]
        assert len(records) == 3, records
        item, equipment, card = records
        assert (item["id"], item["name"], item["price"], item["sell_price"], item["icon_id"], item["stackable"]) == (900001, "Synthetic potion", 20, 10, 11, True), item
        assert (equipment["id"], equipment["name"], equipment["price"], equipment["sell_price"], equipment["durability"], equipment["sex"], equipment["professions"], equipment["bonus_str"], equipment["bonus_luk"]) == (900002, "Synthetic sword", 100, 50, 60, 1, "战士、剑客", 12, -2), equipment
        assert equipment["affixes"] == "Synthetic affix\t+5%", equipment
        assert card["id"] == 900003 and card["stackable"] is False, card
        assert "PRIVATE_" not in json.dumps(records) and "Duplicate" not in json.dumps(records), records
        sql("BEGIN READ ONLY; SET LOCAL ROLE public_catalog_fixture_reader; SELECT * FROM public.accounts; ROLLBACK;\n", expect_error="permission denied for table accounts")
        sql("BEGIN READ ONLY; INSERT INTO public.accounts VALUES ('SHOULD_NEVER_WRITE'); ROLLBACK;\n", expect_error="cannot execute INSERT in a read-only transaction")
        print(json.dumps({"result": "pass", "source": "synthetic fixture only", "records": len(records), "transaction": "repeatable read / read only", "private_table_access": "denied", "writes": "denied", "tcp_or_unix_listener": False}))


if __name__ == "__main__":
    main()
