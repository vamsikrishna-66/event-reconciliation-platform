import os
from typing import Annotated, Literal

import httpx
import psycopg
from fastapi import FastAPI, HTTPException, Path, Request
from psycopg.rows import dict_row
from pydantic import BaseModel, ConfigDict

app = FastAPI()
OrderID = Annotated[str, Path(min_length=1, max_length=128, pattern=r"^[a-zA-Z0-9_-]+$")]
Fact = Literal["status", "expected_cents", "paid_cents", "refunded_cents", "net_cents"]


class Focus(BaseModel):
    model_config = ConfigDict(extra="forbid")
    focus: Fact


@app.get("/tools/orders/{order_id}", name="order_evidence")
def order_evidence(order_id: OrderID):
    try:
        with psycopg.connect(os.getenv("EVIDENCE_DSN", ""), connect_timeout=3,
                             options="-c default_transaction_read_only=on -c statement_timeout=3000",
                             row_factory=dict_row) as db:
            row = db.execute(
                """SELECT order_id, status, expected_cents::text AS expected_cents, paid_cents::text AS paid_cents,
                          refunded_cents::text AS refunded_cents, net_cents::text AS net_cents
                   FROM order_reconciliation WHERE order_id = %s""",
                (order_id,),
            ).fetchone()
    except psycopg.Error as error:
        raise HTTPException(503, "Ledger evidence is unavailable") from error
    if row is None:
        raise HTTPException(404, "Order not found")
    return row


@app.post("/explain/orders/{order_id}")
def explain(order_id: OrderID, request: Request):
    endpoint = os.getenv("LLM_CHAT_URL")
    model = os.getenv("LLM_MODEL")
    if not endpoint or not model:
        raise HTTPException(503, "Explanation model is not configured")
    row = order_evidence(order_id)
    key = os.getenv("LLM_API_KEY")
    try:
        result = httpx.post(endpoint, timeout=15,
                            headers={"Authorization": f"Bearer {key}"} if key else {},
                            json={
                                "model": model,
                                "messages": [
                                    {"role": "system", "content": (
                                        'Choose the amount most relevant to the status. Reply only '
                                        'with a JSON object with key "focus" and one of these values: '
                                        'status, expected_cents, paid_cents, refunded_cents, net_cents.'
                                    )},
                                    {"role": "user", "content": str(row)},
                                ],
                            })
        result.raise_for_status()
        focus = Focus.model_validate_json(result.json()["choices"][0]["message"]["content"]).focus
    except (httpx.HTTPError, ValueError, KeyError, IndexError, TypeError) as error:
        raise HTTPException(502, "Explanation model returned unusable output") from error
    details = {
        "reconciled": "Captured payments match the expected amount.",
        "missing_order": "The ledger has no order creation event.",
        "multiple_orders": "The ledger has multiple order creation events.",
        "refund_exceeds_payment": "Recorded refunds exceed captured payments.",
        "underpaid": "Captured payments are below the expected amount.",
        "overpaid": "Captured payments exceed the expected amount.",
    }
    amounts = {
        "expected_cents": "Expected amount",
        "paid_cents": "Captured payments",
        "refunded_cents": "Recorded refunds",
        "net_cents": "Net after refunds",
    }
    explanation = details[row["status"]]
    if focus in amounts:
        explanation += f" {amounts[focus]}: {row[focus]} cents."
    return {
        "explanation": explanation,
        "focus": focus,
        "evidence": row,
        "evidence_url": str(request.url_for("order_evidence", order_id=order_id)),
    }
