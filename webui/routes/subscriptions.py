from fastapi import APIRouter
from fastapi.concurrency import run_in_threadpool

from webui.schemas.subscriptions import (
    SubscriptionCreateBody,
    SubscriptionTestBody,
    SubscriptionUpdateBody,
)
from webui.services.subscriptions import (
    check_subscription_payload,
    create_subscription_payload,
    delete_subscription_payload,
    get_subscription_payload,
    list_subscriptions_payload,
    test_subscription_payload,
    update_subscription_payload,
)

router = APIRouter()


@router.get("/api/subscriptions")
async def list_subscriptions():
    return await run_in_threadpool(list_subscriptions_payload)


@router.post("/api/subscriptions")
async def create_subscription(body: SubscriptionCreateBody):
    return await run_in_threadpool(create_subscription_payload, body)


@router.post("/api/subscriptions/test")
async def test_subscription(body: SubscriptionTestBody):
    return await run_in_threadpool(test_subscription_payload, body)


@router.get("/api/subscriptions/{subscription_id}")
async def get_subscription(subscription_id: int):
    return await run_in_threadpool(get_subscription_payload, subscription_id)


@router.put("/api/subscriptions/{subscription_id}")
async def update_subscription(subscription_id: int, body: SubscriptionUpdateBody):
    return await run_in_threadpool(update_subscription_payload, subscription_id, body)


@router.delete("/api/subscriptions/{subscription_id}")
async def delete_subscription(subscription_id: int):
    return await run_in_threadpool(delete_subscription_payload, subscription_id)


@router.post("/api/subscriptions/{subscription_id}/check")
async def check_subscription(subscription_id: int):
    return await run_in_threadpool(check_subscription_payload, subscription_id)
