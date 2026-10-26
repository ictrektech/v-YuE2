#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import urllib.request
from pathlib import Path


SPREADSHEET = os.getenv("FEISHU_SPREADSHEET_TOKEN", "Htotsn3oahO1zxt73YMcaB1zn8e")


def request(method: str, url: str, token: str = "", body: dict | None = None) -> dict:
    data = json.dumps(body).encode() if body is not None else None
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    with urllib.request.urlopen(urllib.request.Request(url, data=data, headers=headers, method=method), timeout=30) as response:
        value = json.load(response)
    if value.get("code") != 0:
        raise RuntimeError(value)
    return value


def col_name(index: int) -> str:
    value = ""
    while index:
        index, rem = divmod(index - 1, 26)
        value = chr(65 + rem) + value
    return value


def text(value) -> str:
    if isinstance(value, str):
        return value.strip()
    if isinstance(value, list):
        return "".join(text(part) for part in value).strip()
    if isinstance(value, dict):
        return str(value.get("text") or value.get("link") or "").strip()
    return "" if value is None else str(value).strip()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--sheet", required=True)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--component", default="v-voice")
    parser.add_argument("--repository", default="swr.cn-southwest-2.myhuaweicloud.com/ictrek/v-voice")
    args = parser.parse_args()

    config_path = Path(os.getenv("FEISHU_CONFIG_FILE", str(Path.home() / ".feishu.json")))
    config = json.loads(config_path.read_text())
    login = request(
        "POST",
        "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
        body={"app_id": config["feishu_app_id"], "app_secret": config["feishu_app_secret"]},
    )
    token = login["tenant_access_token"]
    base = f"https://open.feishu.cn/open-apis/sheets"
    sheets = request("GET", f"{base}/v3/spreadsheets/{SPREADSHEET}/sheets/query", token)
    sheet = next(row for row in sheets["data"]["sheets"] if row["title"] == args.sheet)
    sheet_id = sheet["sheet_id"]

    values = request("GET", f"{base}/v2/spreadsheets/{SPREADSHEET}/values/{sheet_id}!A1:ZZ2", token)
    rows = values.get("data", {}).get("valueRange", {}).get("values", [])
    headers = rows[0] if rows else []
    repositories = rows[1] if len(rows) > 1 else []
    column = next((i for i, value in enumerate(headers, 1) if text(value) == args.component), None)
    if column is None:
        column = max([i for row in (headers, repositories) for i, value in enumerate(row, 1) if text(value)] or [0]) + 1
        if column > int(sheet.get("grid_properties", {}).get("column_count", 0)):
            request(
                "POST",
                f"{base}/v2/spreadsheets/{SPREADSHEET}/dimension_range",
                token,
                {"dimension": {"sheetId": sheet_id, "majorDimension": "COLUMNS", "length": 1}},
            )
    column_name = col_name(column)

    dates = request("GET", f"{base}/v2/spreadsheets/{SPREADSHEET}/values/{sheet_id}!A4:A2000", token)
    date_rows = dates.get("data", {}).get("valueRange", {}).get("values", [])
    today = dt.datetime.now().strftime("%Y%m%d")
    row_number = next((index + 4 for index, row in enumerate(date_rows) if row and text(row[0]) == today), None)
    if row_number is None:
        request(
            "POST",
            f"{base}/v2/spreadsheets/{SPREADSHEET}/values_prepend",
            token,
            {"valueRange": {"range": f"{sheet_id}!A4:A4", "values": [[today]]}},
        )
        row_number = 4

    request(
        "PUT",
        f"{base}/v2/spreadsheets/{SPREADSHEET}/values",
        token,
        {
            "valueRange": {
                "range": f"{sheet_id}!{column_name}1:{column_name}2",
                "values": [[args.component], [args.repository]],
            }
        },
    )
    request(
        "PUT",
        f"{base}/v2/spreadsheets/{SPREADSHEET}/values",
        token,
        {
            "valueRange": {
                "range": f"{sheet_id}!{column_name}{row_number}:{column_name}{row_number}",
                "values": [[args.tag]],
            }
        },
    )
    print(f"Feishu updated: {args.sheet}!{column_name}{row_number} = {args.tag}")


if __name__ == "__main__":
    main()
