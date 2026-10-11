#!/usr/bin/env python3
"""Socket-free real PostgreSQL checks. PG_BIN must name trusted PostgreSQL bin.
Creates a new disposable cluster below --work-dir; never targets an existing DB.
Example: PG_BIN=/usr/lib/postgresql/17/bin python3 server/tests/sql/validate_daoist_postgres.py --work-dir /tmp/daoist-pg-check
"""
import argparse
import os
from pathlib import Path
import re
import subprocess
import shutil

REPO = Path(__file__).resolve().parents[3]


# The init-N files are raw chunks, not individually executable SQL scripts.
# Concatenate on disk so even a quote/comment/dollar tag split across files is
# lexed exactly as it is by the production migrate loader.
NORMAL_TOKEN = re.compile(r"--|/\*|'|\"|\$(?:[^\W\d]\w*)?\$|;", re.UNICODE)
COMMENT_TOKEN = re.compile(r'/\*|\*/')
ESCAPE_TOKEN = re.compile(r"['\\]")


def write_single_user_commands(source, target):
    """Stream SQL into postgres -j commands, using only a line-sized buffer.

    Semicolons inside strings, quoted identifiers, comments and dollar-quoted
    bodies never end a command. Dollar strings become equivalent E strings so
    PostgreSQL's *unconditional* ;\\n\\n input delimiter cannot split DO bodies.
    Ordinary strings and quoted identifiers remain byte-for-byte unchanged;
    reject an embedded single-user delimiter instead of silently altering data.
    This deliberately handles the SQL used by these migrations, not psql COPY
    stdin data or SQL-standard BEGIN ATOMIC routine bodies.
    """
    state = 'normal'
    tag = None
    depth = 0
    has_sql = False
    statements = 0
    statement_size = 0
    largest_statement = 0
    quoted_tail = ''

    def emit(text, literal=False, dollar=False, comment=False):
        nonlocal quoted_tail, statement_size
        statement_size += len(text)
        if dollar:
            target.write(text.replace('\\', '\\\\').replace("'", "''")
                         .replace('\r', '\\r').replace('\n', '\\n'))
        else:
            if literal and ';\n\n' in quoted_tail + text:
                raise ValueError('literal contains the postgres -j delimiter ;\\n\\n')
            target.write(text.replace(';', '; ') if comment else text)
        quoted_tail = (quoted_tail + text)[-2:] if literal else ''

    for line_number, line in enumerate(source, 1):
        pos = 0
        while pos < len(line):
            if state == 'dollar':
                end = line.find(tag, pos)
                if end < 0:
                    emit(line[pos:], dollar=True)
                    break
                emit(line[pos:end], dollar=True)
                target.write("'")
                statement_size += len(tag)
                pos = end + len(tag)
                state = 'normal'
                continue
            if state in ('single', 'escape', 'double'):
                quote = '"' if state == 'double' else "'"
                match = ESCAPE_TOKEN.search(line, pos) if state == 'escape' else None
                end = (match.start() if match else -1) if state == 'escape' else line.find(quote, pos)
                if end < 0:
                    emit(line[pos:], literal=True)
                    break
                emit(line[pos:end + 1], literal=True)
                pos = end + 1
                if state == 'escape' and line[end] == '\\':
                    if pos < len(line):
                        emit(line[pos:pos + 1], literal=True)
                        pos += 1
                elif pos < len(line) and line[pos] == quote:
                    emit(line[pos:pos + 1], literal=True)
                    pos += 1
                else:
                    state = 'normal'
                    quoted_tail = ''
                continue
            if state == 'block':
                match = COMMENT_TOKEN.search(line, pos)
                if not match:
                    emit(line[pos:], comment=True)
                    break
                emit(line[pos:match.end()], comment=True)
                depth += 1 if match.group() == '/*' else -1
                if depth == 0:
                    state = 'normal'
                pos = match.end()
                continue
            match = NORMAL_TOKEN.search(line, pos)
            if not match:
                remainder = line[pos:]
                has_sql = has_sql or bool(remainder.strip())
                emit(remainder)
                break
            prefix = line[pos:match.start()]
            has_sql = has_sql or bool(prefix.strip())
            emit(prefix)
            token = match.group()
            pos = match.end()
            if token == '--':
                emit(line[match.start():], comment=True)
                break
            if token == '/*':
                emit(token, comment=True)
                state, depth = 'block', 1
            elif token == ';':
                emit(token)
                target.write('\n\n')
                statements += 1
                largest_statement = max(largest_statement, statement_size)
                statement_size = 0
                has_sql = False
            elif token.startswith('$'):
                # A dollar sign is legal inside unquoted identifiers. Such a
                # token is not a string opener unless separated from its name.
                previous = line[match.start() - 1:match.start()] if match.start() else ''
                if previous and (previous.isalnum() or previous in '_$'):
                    emit(token)
                else:
                    target.write("E'")
                    statement_size += len(token)
                    state, tag = 'dollar', token
                has_sql = True
            else:
                emit(token, literal=True)
                has_sql = True
                if token == '"':
                    state = 'double'
                else:
                    start = match.start()
                    escape = start > 0 and line[start - 1] in 'eE' and (
                        start == 1 or not (line[start - 2].isalnum() or line[start - 2] in '_$'))
                    state = 'escape' if escape else 'single'
    if state != 'normal':
        raise ValueError(f'unterminated {state} at source line {line_number}')
    if has_sql:
        raise ValueError('SQL source must end at a complete semicolon-terminated statement')
    target.write('\n')
    return statements, largest_statement


def concatenate_sources(paths, target):
    for path in paths:
        with path.open('rb') as source:
            shutil.copyfileobj(source, target, length=64 * 1024)


def check_splitter():
    """No PostgreSQL needed; cover all lexical states and arbitrary cuts."""
    from io import BytesIO, StringIO, TextIOWrapper
    from tempfile import TemporaryDirectory
    example = (
        "-- comment ;\n\n/* outer ; /* nested ; */ end */\n"
        "INSERT INTO t VALUES('first; quote '' kept', E'escaped \\' ; kept'),\n"
        "('跨分片 value');\n"
        'SELECT "column;name", $body$a;\n\nb\'\\c$body$;\n'
        'DO $$ BEGIN\n PERFORM 1;\n\n PERFORM 2;\n END $$;\n'
    )
    expected = StringIO()
    stats = write_single_user_commands(StringIO(example), expected)
    assert stats[0] == 3, stats
    assert expected.getvalue().count(';\n\n') == 3
    assert "E'a;\\n\\nb''\\\\c'" in expected.getvalue()
    with TemporaryDirectory(prefix='daoist-sql-boundaries-') as directory:
        # Every possible two-fragment boundary, including quoted text, comment
        # openers/closers, E escapes and dollar tags, must give identical output.
        first, second = Path(directory) / 'first.sql', Path(directory) / 'second.sql'
        encoded = example.encode('utf-8')
        for cut in range(len(encoded) + 1):
            first.write_bytes(encoded[:cut])
            second.write_bytes(encoded[cut:])
            joined = BytesIO()
            concatenate_sources([first, second], joined)
            joined.seek(0)
            actual = StringIO()
            assert write_single_user_commands(TextIOWrapper(joined, encoding='utf-8', newline=''), actual) == stats
            assert actual.getvalue() == expected.getvalue(), cut
    for invalid in ("SELECT 'unclosed;", 'DO $body$ BEGIN; END;', '/* open', 'SELECT 1', "SELECT 'a;\n\nb';"):
        try:
            write_single_user_commands(StringIO(invalid), StringIO())
        except ValueError:
            pass
        else:
            raise AssertionError(f'invalid or unsafe SQL accepted: {invalid!r}')
    print('PASS SQL splitter source-boundary checks', flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--work-dir', type=Path)
    parser.add_argument('--self-test', action='store_true', help='run only the SQL splitter regression checks')
    args = parser.parse_args()
    check_splitter()
    if args.self_test:
        return
    if args.work_dir is None:
        parser.error('--work-dir is required unless --self-test is used')
    work = args.work_dir.resolve()
    if work.exists():
        parser.error('--work-dir must not exist (a fresh disposable directory is required)')
    bins = Path(os.environ['PG_BIN'])
    work.mkdir(parents=True)
    data = work / 'data'
    subprocess.run([str(bins / 'initdb'), '-D', str(data), '--no-locale', '-E', 'UTF8', '-U', 'validator', '-A', 'trust'], check=True, stdout=(work / 'initdb.log').open('w'), stderr=subprocess.STDOUT)

    def run(database, label, sql=None, paths=None, transaction=False):
        input_path = work / f'{label}.sql'
        raw_path = work / f'{label}.source.sql'
        log_path = work / f'{label}.log'
        # Never materialize the complete init or a complete INSERT in Python.
        # PostgreSQL releases each command's parser tree while the surrounding
        # transaction retains the production init's all-or-nothing semantics.
        with raw_path.open('wb') as raw:
            if transaction:
                raw.write(b'BEGIN;\n')
            if paths is not None:
                concatenate_sources(paths, raw)
            else:
                raw.write(sql.encode('utf-8'))
            if transaction:
                raw.write(b'\nCOMMIT;\n')
        with raw_path.open('r', encoding='utf-8', newline='') as source, input_path.open('w', encoding='utf-8', newline='') as target:
            count, largest = write_single_user_commands(source, target)
        raw_path.unlink()
        print(f'RUN {label}: {count} statements; largest {largest:,} characters', flush=True)
        with input_path.open('r', encoding='utf-8') as source, log_path.open('w', encoding='utf-8') as log:
            result = subprocess.run([str(bins / 'postgres'), '--single', '-j', '-D', str(data), database], stdin=source, stdout=log, stderr=subprocess.STDOUT)
        # A single-user backend can exit zero after SQL errors; inspect the log.
        # Scan line by line instead of loading potentially enormous error text.
        with log_path.open(encoding='utf-8') as log:
            errors = any(re.search(r'\b(?:ERROR|FATAL|PANIC):', line) for line in log)
        if result.returncode or errors:
            raise RuntimeError(f'{label} failed (exit {result.returncode}); see {log_path}')
        input_path.unlink()
        print(f'PASS {label}', flush=True)
        return log_path

    def require_marker(log_path, marker):
        with log_path.open(encoding='utf-8') as log:
            if not any(marker in line for line in log):
                raise AssertionError(f'missing {marker}; see {log_path}')

    # Verify the scanner's escaped dollar bodies preserve actual SQL values,
    # including nested quotes, backslashes, empty lines and Unicode.
    run('postgres', 'splitter-postgres-semantics', r"""
BEGIN;
CREATE TEMP TABLE "semicolon;name" (value text);
/* outer ;

   /* nested ; */ still a comment */
INSERT INTO "semicolon;name" VALUES ($tag$跨片;'\

next$tag$), (E'escaped \' ; kept');
DO $body$ BEGIN
 IF NOT EXISTS (SELECT FROM "semicolon;name" WHERE value = E'跨片;''\\\n\nnext') THEN
  RAISE EXCEPTION 'dollar string value changed';
 END IF;

 IF NOT EXISTS (SELECT FROM "semicolon;name" WHERE value = 'escaped '' ; kept') THEN
  RAISE EXCEPTION 'escaped quote value changed';
 END IF;
END $body$;
ROLLBACK;
SELECT 'SQL_SPLITTER_POSTGRES_PASS';
""")

    # CREATE DATABASE cannot share an implicit transaction with a second command.
    run('postgres', 'create-fresh', 'CREATE DATABASE fresh_test;')
    run('postgres', 'create-upgrade', 'CREATE DATABASE upgrade_test;')
    fragments = [REPO / f'server/migrations/000000_init-{i}.sql' for i in range(52)]
    run('fresh_test', 'fresh-init', paths=fragments, transaction=True)
    assertions = REPO / 'server/tests/sql/daoist_endgame_assertions.sql'
    result = run('fresh_test', 'fresh-assertions', paths=[assertions])
    require_marker(result, 'DAOIST_ENDGAME_SQL_PASS')
    run('upgrade_test', 'baseline-init', paths=fragments[:50], transaction=True)
    run('upgrade_test', 'seed-existing-character', "INSERT INTO accounts(id,username) VALUES(930000001,'pre_upgrade'); INSERT INTO characters(id,account_id,slot,name,race,map_id,pos_x,pos_y) VALUES(930000001,930000001,0,'pre_upgrade',1,7,10,20);")
    migrations = [REPO / 'docs/migrations/20261011_daoist_endgame_v1.sql',
                  REPO / 'docs/migrations/20261011_item_local_enhancement_v4.sql']
    # Exercise an already-installed original v1 schema, not only a pre-v1 DB.
    run('upgrade_test', 'seed-original-unique-only-schema', """
CREATE TABLE equipment_endgame_state (
 uid BIGINT PRIMARY KEY REFERENCES item_instances(uid) ON DELETE CASCADE,
 revision BIGINT NOT NULL CHECK(revision >= 0),
 unique_id INTEGER NOT NULL CHECK(unique_id > 0),
 unique_roll_bp INTEGER NOT NULL CHECK(unique_roll_bp BETWEEN 8000 AND 12000)
);
INSERT INTO item_instances(uid,item_id,instance_kind) VALUES(930000009,32001,1);
INSERT INTO equipment_instances(uid,refine_level,socket_count,socket_item_ids,rolled_affix_count,rolled_card_ids,rolled_attrs,rolled_values,rolled_modes,wash_quality,wash_count,wash_attrs,wash_values,wash_modes)
 VALUES(930000009,6,2,ARRAY[101,102,0,0,0],2,ARRAY[301,302,0,0,0],ARRAY[10,20,0,0,0],ARRAY[137,983,0,0,0],ARRAY[0,1,0,0,0],2,2,ARRAY[30,40,0,0],ARRAY[57,119,0,0],ARRAY[0,1,0,0]);
INSERT INTO equipment_endgame_state(uid,revision,unique_id,unique_roll_bp) VALUES(930000009,9,32001,9000);
CREATE TABLE validation_legacy_item_before AS
 SELECT to_jsonb(i) AS item, to_jsonb(e) AS equipment
 FROM item_instances i JOIN equipment_instances e USING(uid) WHERE i.uid=930000009;
""")
    run('upgrade_test', 'upgrade-first', paths=migrations)
    run('upgrade_test', 'seed-endgame-persistence', """
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM characters WHERE id=930000001 AND trial_key_reward=0 AND trial_completion_id='') THEN RAISE EXCEPTION 'upgrade defaults missing'; END IF;
END $$;
UPDATE characters SET trial_key_reward=7,trial_completion_id='persisted-completion' WHERE id=930000001;
INSERT INTO item_instances(uid,item_id,instance_kind) VALUES(930000001,32001,1);
INSERT INTO equipment_instances(uid,refine_level) VALUES(930000001,4);
INSERT INTO equipment_endgame_state(uid,revision,unique_id,unique_roll_bp) VALUES(930000001,2,32001,11000);
INSERT INTO item_instances(uid,item_id,instance_kind) VALUES(930000002,1001,1);
INSERT INTO equipment_instances(uid,socket_count) VALUES(930000002,2);
INSERT INTO equipment_endgame_state(uid,revision,unique_id,unique_roll_bp) VALUES(930000002,2,0,0);
UPDATE equipment_endgame_state SET formation_roll_bp=10417 WHERE uid=930000001;
INSERT INTO character_endgame_state VALUES(930000001,3,'{"keys":7}');
INSERT INTO endgame_receipts VALUES(930000001,'persisted-request','claim',repeat('c',64),3,'{"ok":true}');
INSERT INTO endgame_runs VALUES('persisted-run',2,'completed','{"result":"won"}');
CREATE TABLE validation_schema_before AS SELECT md5(string_agg(table_name||column_name||data_type||is_nullable||coalesce(column_default,''),',' ORDER BY table_name,ordinal_position)) AS fingerprint FROM information_schema.columns WHERE table_schema='public' AND (table_name LIKE '%endgame%' OR table_name='characters');
""")
    run('upgrade_test', 'upgrade-second', paths=migrations)
    # Every run() starts a new standalone process and reads the on-disk database.
    run('upgrade_test', 'restart-persistence-idempotence', """
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM characters WHERE id=930000001 AND trial_key_reward=7 AND trial_completion_id='persisted-completion') THEN RAISE EXCEPTION 'trial persistence'; END IF;
 IF NOT EXISTS(SELECT FROM equipment_endgame_state WHERE uid=930000001 AND revision=2 AND unique_roll_bp=11000 AND formation_roll_bp=10417) THEN RAISE EXCEPTION 'overlay persistence'; END IF;
 IF NOT EXISTS(SELECT FROM equipment_endgame_state WHERE uid=930000002 AND revision=2 AND unique_id=0 AND unique_roll_bp=0) THEN RAISE EXCEPTION 'ordinary revision persistence'; END IF;
 IF NOT EXISTS(SELECT FROM equipment_endgame_state WHERE uid=930000009 AND revision=9 AND unique_id=32001 AND unique_roll_bp=9000 AND formation_roll_bp=0) THEN RAISE EXCEPTION 'original overlay preserved'; END IF;
 IF NOT EXISTS(SELECT FROM validation_legacy_item_before b JOIN item_instances i ON i.uid=930000009 JOIN equipment_instances e USING(uid) WHERE b.item=to_jsonb(i) AND b.equipment=to_jsonb(e)) THEN RAISE EXCEPTION 'legacy UID/native raw rolls changed'; END IF;
 IF NOT EXISTS(SELECT FROM equipment_instances WHERE uid=930000001 AND refine_level=4) THEN RAISE EXCEPTION 'native item persistence'; END IF;
 IF NOT EXISTS(SELECT FROM character_endgame_state WHERE char_id=930000001 AND revision=3 AND payload->>'keys'='7') THEN RAISE EXCEPTION 'state persistence'; END IF;
 IF NOT EXISTS(SELECT FROM endgame_receipts WHERE char_id=930000001 AND request_id='persisted-request' AND response->>'ok'='true') THEN RAISE EXCEPTION 'receipt persistence'; END IF;
 IF NOT EXISTS(SELECT FROM endgame_runs WHERE run_id='persisted-run' AND revision=2 AND state='completed' AND payload->>'result'='won') THEN RAISE EXCEPTION 'run persistence'; END IF;
 IF (SELECT fingerprint FROM validation_schema_before) IS DISTINCT FROM (SELECT md5(string_agg(table_name||column_name||data_type||is_nullable||coalesce(column_default,''),',' ORDER BY table_name,ordinal_position)) FROM information_schema.columns WHERE table_schema='public' AND (table_name LIKE '%endgame%' OR table_name='characters')) THEN RAISE EXCEPTION 'second upgrade changed schema'; END IF;
END $$;
SELECT 'DAOIST_ENDGAME_RESTART_PASS';
""")
    result = run('upgrade_test', 'upgrade-assertions', paths=[assertions])
    require_marker(result, 'DAOIST_ENDGAME_SQL_PASS')
    print('All SQL checks passed. pgx/network/concurrent-lock tests were NOT run.')


if __name__ == '__main__':
    main()
