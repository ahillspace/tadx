"""Build disposable native fixtures from public, deterministic data.

Install tableauhyperapi into the selected Python environment before running.
No Tableau credentials or remote content are used.
"""
import argparse
import datetime
import json
from pathlib import Path
from zipfile import ZIP_DEFLATED, ZipFile

from tableauhyperapi import (
    Connection, CreateMode, HyperProcess, Inserter, SqlType, TableDefinition,
    TableName, Telemetry,
)


def generate(output: Path) -> None:
    output.mkdir(parents=True, exist_ok=True)
    for variant, rows in {
        "a": [(1, datetime.date(2026, 1, 1), "East", 10.0),
              (2, datetime.date(2026, 1, 2), "West", 25.0)],
        "b": [(3, datetime.date(2026, 1, 3), "North", 47.0)],
    }.items():
        hyper_file = output / f"fixture-{variant}.hyper"
        table = TableDefinition(TableName("Extract", "Extract"), [
            TableDefinition.Column("ID", SqlType.big_int()),
            TableDefinition.Column("Order Date", SqlType.date()),
            TableDefinition.Column("Region", SqlType.text()),
            TableDefinition.Column("Sales", SqlType.double()),
        ])
        with HyperProcess(Telemetry.DO_NOT_SEND_USAGE_DATA_TO_TABLEAU, parameters={"log_config": ""}) as hyper:
            with Connection(hyper.endpoint, str(hyper_file), CreateMode.CREATE_AND_REPLACE) as conn:
                conn.catalog.create_schema("Extract")
                conn.catalog.create_table(table)
                with Inserter(conn, table) as inserter:
                    inserter.add_rows(rows)
                    inserter.execute()
        columns = """<column datatype='integer' name='[ID]' role='dimension' type='ordinal'/>
<column datatype='date' name='[Order Date]' role='dimension' type='ordinal'/>
<column datatype='string' name='[Region]' role='dimension' type='nominal'/>
<column datatype='real' name='[Sales]' role='measure' type='quantitative'/>"""
        source = f"""<datasource caption='Live fixture {variant}' inline='true' name='live-fixture' version='18.1'>
<connection class='hyper' dbname='Data/Extracts/fixture.hyper' schema='Extract'>
<relation name='Extract' table='[Extract].[Extract]' type='table'/>
</connection>{columns}<extract count='-1' enabled='true' units='records'>
<connection class='hyper' dbname='Data/Extracts/fixture.hyper' schema='Extract'>
<relation name='Extract' table='[Extract].[Extract]' type='table'/>
</connection></extract></datasource>"""
        workbook = f"""<?xml version='1.0' encoding='utf-8'?>
<workbook original-version='18.1' version='18.1'>
<datasources>{source}</datasources><worksheets><worksheet name='Sales by region'>
<table><view><datasources><datasource caption='Live fixture {variant}' name='live-fixture'/></datasources>
<datasource-dependencies datasource='live-fixture'>{columns}
<column-instance column='[Region]' derivation='None' name='[none:Region:nk]' pivot='key' type='nominal'/>
<column-instance column='[Sales]' derivation='Sum' name='[sum:Sales:qk]' pivot='key' type='quantitative'/>
</datasource-dependencies></view><style/><panes><pane selection-relaxation-option='selection-relaxation-allow'>
<view><breakdown value='auto'/></view><mark class='Bar'/><encodings/></pane></panes>
<rows>[live-fixture].[none:Region:nk]</rows><cols>[live-fixture].[sum:Sales:qk]</cols></table>
</worksheet></worksheets><windows><window class='worksheet' name='Sales by region'/></windows></workbook>"""
        for extension, primary, document in [
            ("tdsx", "fixture.tds", source), ("twbx", "fixture.twb", workbook)
        ]:
            with ZipFile(output / f"fixture-{variant}.{extension}", "w", ZIP_DEFLATED) as package:
                package.writestr(primary, document)
                package.write(hyper_file, "Data/Extracts/fixture.hyper")
        manifest = {
            "columns": ["[ID]", "[Order Date]", "[Region]", "[Sales]"],
            "worksheets": ["Sales by region"],
            "extract_count": 1,
            "rows": [[r[0], str(r[1]), r[2], r[3]] for r in rows],
        }
        (output / f"fixture-{variant}.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output", type=Path)
    generate(parser.parse_args().output)
