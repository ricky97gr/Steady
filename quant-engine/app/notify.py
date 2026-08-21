"""飞书自定义机器人推送（统一通知传输层）

- 配置读 app_config 表（页面可改，值以库为准）；不读环境变量，未配置即禁用
- 异步发送（daemon 线程），失败不阻塞调用方；有限重试 + 超时
- webhook 日志脱敏，绝不打印完整 token
- 卡片结构：interactive 富文本，标题 + lark_md 摘要 + Dashboard 链接

配置键（app_config）：feishu.enabled / feishu.webhook_url / feishu.dashboard_url /
feishu.timeout / feishu.max_retries / feishu.secret / feishu.at_all（通知卡片 @所有人）
"""
import base64
import hashlib
import hmac
import json
import logging
import threading
import time
import urllib.request
from urllib.parse import urlsplit

from sqlalchemy import select

from app.models.tables import AppConfig

logger = logging.getLogger("notify")

# app_config 键 → 内部配置键
_KEYMAP = {
    "feishu.enabled": "enabled",
    "feishu.webhook_url": "webhook",
    "feishu.dashboard_url": "dashboard",
    "feishu.timeout": "timeout",
    "feishu.max_retries": "max_retries",
    "feishu.secret": "secret",
    "feishu.at_all": "at_all",
}


def gen_sign(secret: str, timestamp: int) -> str:
    """飞书机器人签名：sign = base64(hmac_sha256(key=f"{ts}\n{secret}", msg=b""))

    官方算法（机器人开启签名校验后必填，服务端用同样算法验签）：
    string_to_sign = "{timestamp}\n{secret}" 作为 HMAC 密钥，空消息体。
    """
    string_to_sign = f"{timestamp}\n{secret}"
    hmac_code = hmac.new(string_to_sign.encode("utf-8"),
                         digestmod=hashlib.sha256).digest()
    return base64.b64encode(hmac_code).decode("utf-8")


def load_config(db=None) -> dict:
    """加载飞书配置：仅从 app_config 表读取（页面可改，值以库为准）

    不读环境变量——按「业务配置全走页面，env 层只保留数据库凭据」原则；
    库未配置或读取失败 → enabled=False（通知不发送，不阻塞主业务）。
    """
    vals = {k: "" for k in _KEYMAP.values()}
    if db is not None:
        try:
            for row in db.execute(select(AppConfig)).scalars():
                internal = _KEYMAP.get(row.key)
                if internal and row.value not in (None, ""):
                    vals[internal] = row.value
        except Exception:
            db.rollback()
            logger.warning("读取 app_config 失败，按未配置处理", exc_info=True)
    return {
        "enabled": str(vals["enabled"]).strip().lower() in ("1", "true", "yes", "on"),
        "webhook": (vals["webhook"] or "").strip(),
        "dashboard": (vals["dashboard"] or "http://localhost").strip().rstrip("/"),
        "timeout": float(vals["timeout"] or 10),
        "max_retries": int(vals["max_retries"] or 2),
        "secret": (vals["secret"] or "").strip(),
        "at_all": str(vals["at_all"]).strip().lower() in ("1", "true", "yes", "on"),
    }


class FeishuNotifier:
    """飞书卡片推送器（一次性使用：传入配置，send_card 异步发送）"""

    def __init__(self, cfg: dict):
        self.enabled = bool(cfg.get("enabled"))
        self.webhook = str(cfg.get("webhook") or "").strip()
        self.dashboard = str(cfg.get("dashboard") or "http://localhost").rstrip("/")
        self.timeout = float(cfg.get("timeout") or 10)
        self.max_retries = int(cfg.get("max_retries") or 2)
        self.secret = str(cfg.get("secret") or "").strip()
        self.at_all = bool(cfg.get("at_all"))

    def url(self, path: str = "/") -> str:
        return self.dashboard + (path if path.startswith("/") else "/" + path)

    @property
    def ready(self) -> bool:
        """可发送：启用 且 已配置 webhook"""
        return self.enabled and bool(self.webhook)

    def mask(self) -> str:
        try:
            parts = urlsplit(self.webhook)
            return f"{parts.scheme}://{parts.netloc}{parts.path}?****"
        except Exception:
            return "****"

    def send_card(self, title: str, content_md: str, template: str = "blue",
                  footer: str | None = None, wait: bool = False) -> bool:
        """发送富文本卡片。默认异步（不阻塞主流程）；wait=True 同步并返回是否送达。"""
        if not self.ready:
            logger.debug("飞书通知未启用或未配置 webhook，跳过推送")
            return False
        payload = self._build_card(title, content_md, template, footer)
        if self.secret:  # 开启签名校验：请求体携带 timestamp + sign
            ts = int(time.time())
            payload["timestamp"] = str(ts)
            payload["sign"] = gen_sign(self.secret, ts)
        if wait:
            return self._post(payload)
        threading.Thread(target=self._post, args=(payload,), daemon=True).start()
        return True

    def send_test(self, wait: bool = True) -> bool:
        content = (
            "**测试卡片**\n\n如果能看到这条消息，说明飞书机器人配置正常。\n\n"
            f"[打开 Dashboard]({self.url('/')})"
        )
        return self.send_card("Steady · 测试通知", content,
                              template="green", footer="测试推送", wait=wait)

    def _build_card(self, title: str, content_md: str, template: str,
                    footer: str | None) -> dict:
        if self.at_all:  # @所有人：lark_md 内容前置 @ 标签，触发全员提及通知
            content_md = '<at id="all"></at>\n\n' + content_md
        elements = [
            {"tag": "div", "text": {"tag": "lark_md", "content": content_md}},
            {"tag": "hr"},
        ]
        if footer:
            elements.append({"tag": "note",
                             "elements": [{"tag": "plain_text", "content": footer}]})
        return {
            "msg_type": "interactive",
            "card": {
                "config": {"wide_screen_mode": True},
                "header": {"template": template,
                           "title": {"tag": "plain_text", "content": title}},
                "elements": elements,
            },
        }

    def _post(self, payload: dict) -> bool:
        data = json.dumps(payload).encode("utf-8")
        req = urllib.request.Request(
            self.webhook, data=data,
            headers={"Content-Type": "application/json"})
        for attempt in range(1, self.max_retries + 1):
            try:
                with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                    body = json.loads(resp.read().decode("utf-8") or "{}")
                if body.get("code") == 0:
                    logger.info("飞书推送成功（%s）", self.mask())
                    return True
                logger.warning("飞书推送失败 第%d/%d次: HTTP %s code=%s msg=%s",
                               attempt, self.max_retries, resp.status,
                               body.get("code"), body.get("msg", ""))
            except Exception as e:  # 超时/连接错误等：任何异常都不上抛
                logger.warning("飞书推送异常 第%d/%d次: %s", attempt,
                               self.max_retries, e)
            if attempt < self.max_retries:
                time.sleep(attempt)
        logger.error("飞书推送失败，已达最大重试次数（%s）", self.mask())
        return False
