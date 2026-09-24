"""SMTP and log mailers; ``build_message`` renders a plain-text RFC 5322 message."""

from __future__ import annotations

import datetime as dt
import logging
import smtplib
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Protocol

import anyio

from ...definition.common import is_bare_email
from ...settings import Settings

log = logging.getLogger("openforms.mail")


class Mailer(Protocol):
    async def send(self, to: str, subject: str, body: str) -> None: ...


class LogMailer:
    """Logs instead of sending (no SMTP host configured)."""

    async def send(self, to: str, subject: str, body: str) -> None:
        log.info("email (log mailer) to=%s subject=%s body=%s", to, subject, body)


SendFunc = Callable[[str, int, "tuple[str, str] | None", str, list[str], bytes], None]


def _smtp_send(host: str, port: int, auth: tuple[str, str] | None, from_: str, to: list[str], msg: bytes) -> None:
    with smtplib.SMTP(host, port, timeout=30) as s:
        s.ehlo()
        if s.has_extn("starttls"):
            s.starttls()
            s.ehlo()
        if auth is not None:
            s.login(*auth)
        s.sendmail(from_, to, msg)


@dataclass
class SMTPMailer:
    host: str
    port: int
    from_: str
    username: str = ""
    password: str = ""
    send_func: SendFunc = field(default=_smtp_send)

    @property
    def addr(self) -> str:
        return f"{self.host}:{self.port}"

    async def send(self, to: str, subject: str, body: str) -> None:
        msg = build_message(self.from_, to, subject, body, dt.datetime.now(dt.UTC))
        auth = (self.username, self.password) if self.username else None
        try:
            await anyio.to_thread.run_sync(self.send_func, self.host, self.port, auth, self.from_, [to], msg)
        except Exception as e:
            raise RuntimeError(f"smtp send: {e}") from e


def new_mailer(settings: Settings) -> Mailer:
    if not settings.smtp_host:
        return LogMailer()
    return SMTPMailer(
        host=settings.smtp_host,
        port=settings.smtp_port,
        from_=settings.smtp_from,
        username=settings.smtp_username,
        password=settings.smtp_password,
    )


def _needs_encoding(s: str) -> bool:
    return any(not (" " <= c <= "~" or c == "\t") for c in s)


def q_encode(s: str) -> str:
    """RFC 2047 Q-encoding as Go's ``mime.QEncoding.Encode("utf-8", s)`` does it."""
    if not _needs_encoding(s):
        return s
    prefix, suffix, max_word = "=?utf-8?q?", "?=", 75
    words: list[str] = []
    cur = ""
    for ch in s:
        enc = ""
        for b in ch.encode("utf-8"):
            if b == 0x20:
                enc += "_"
            elif 0x21 <= b <= 0x7E and b not in (ord("="), ord("?"), ord("_")):
                enc += chr(b)
            else:
                enc += f"={b:02X}"
        if len(prefix) + len(cur) + len(enc) + len(suffix) > max_word:
            words.append(prefix + cur + suffix)
            cur = ""
        cur += enc
    words.append(prefix + cur + suffix)
    return " ".join(words)


def build_message(from_: str, to: str, subject: str, body: str, date: dt.datetime) -> bytes:
    """Reject addresses containing CR/LF and strip CR/LF from the subject (no header injection)."""
    for a in (from_, to):
        if "\r" in a or "\n" in a:
            raise ValueError("email address contains a line break")
    if not is_bare_email(to):
        raise ValueError(f"invalid recipient {to!r}")
    subject = subject.replace("\r\n", " ").replace("\r", " ").replace("\n", " ")
    body = body.replace("\r\n", "\n").replace("\n", "\r\n")
    lines = [
        f"From: {from_}",
        f"To: {to}",
        f"Subject: {q_encode(subject)}",
        f"Date: {date.strftime('%a, %d %b %Y %H:%M:%S %z')}",
        "MIME-Version: 1.0",
        "Content-Type: text/plain; charset=utf-8",
        "Content-Transfer-Encoding: 8bit",
        "",
    ]
    return ("\r\n".join(lines) + "\r\n" + body).encode("utf-8")
