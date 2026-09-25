"""Canonical YAML for pulled and scaffolded definitions.

Output is byte-identical to the Go CLI, which rendered the JSON form of a
definition through gopkg.in/yaml.v3 (indent 2, unlimited width). The emitter
below implements the subset of libyaml's block emitter that such documents
use, including its scalar style rules, so strings like ``yes``, ``123`` or
``null`` stay quoted and multi-line text becomes a literal block.
"""

from __future__ import annotations

import math
import re
from decimal import Decimal
from typing import Any

# Keys not listed sort after these, alphabetically.
KEY_ORDER = [
    "slug", "key", "value", "type", "title", "label", "description", "workflow", "initial",
    "settings", "public", "submitLabel", "confirmationMessage", "required", "placeholder", "help",
    "options", "validation", "minLength", "maxLength", "pattern", "min", "max",
    "showIf", "field", "equals", "notEquals", "in",
    "from", "to", "guard", "roles", "requireFields", "actions",
    "url", "subject", "body", "user", "role", "color", "terminal",
    "states", "fields", "onSubmit", "transitions",
]  # fmt: skip
_RANK = {k: i for i, k in enumerate(KEY_ORDER)}
INDENT = 2


def _key_sort(k: str) -> tuple[int, int, str]:
    r = _RANK.get(k)
    return (0, r, "") if r is not None else (1, 0, k)


def canonical_yaml(tree: Any) -> str:
    """Render a JSON-shaped value (dicts, lists, str, int, float, bool, None) as YAML."""
    e = _Emitter()
    e.node(tree, root=True)
    e.write_indent()  # document end
    return "".join(e.out)


# --- scalar resolution (yaml.v3 resolve.go / encode.go) ---------------------------

_RESOLVE_MAP = {
    "true", "True", "TRUE", "false", "False", "FALSE", "", "~", "null", "Null", "NULL",
    ".nan", ".NaN", ".NAN", ".inf", ".Inf", ".INF", "+.inf", "+.Inf", "+.INF", "-.inf", "-.Inf", "-.INF", "<<",
}  # fmt: skip
_OLD_BOOL = {"y", "Y", "yes", "Yes", "YES", "on", "On", "ON", "n", "N", "no", "No", "NO", "off", "Off", "OFF"}
_BASE60 = re.compile(r"^[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+(?:\.[0-9_]*)?$")
_YAML_FLOAT = re.compile(r"^[-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?$")
_GO_DEC_FLOAT = re.compile(r"^[-+]?(\.[0-9]+|[0-9]+\.?[0-9]*)([eE][-+]?[0-9]+)?$")
_GO_HEX_FLOAT = re.compile(r"^[-+]?0[xX]([0-9a-fA-F]+\.?[0-9a-fA-F]*|\.[0-9a-fA-F]+)[pP][-+]?[0-9]+$")
_TS_DATE = re.compile(r"^(\d{4})-(\d{1,2})-(\d{1,2})$")
_TS_FULL = re.compile(
    r"^(\d{4})-(\d{1,2})-(\d{1,2})(?:[Tt](\d{1,2}):(\d{1,2}):(\d{1,2})(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})"
    r"| (\d{1,2}):(\d{1,2}):(\d{1,2})(?:\.\d+)?)$"
)
_INT64 = (-(2**63), 2**63 - 1)


def _go_parse_int(s: str) -> bool:
    """strconv.ParseInt/ParseUint(s, 0, 64) succeeds (underscores already removed)."""
    m = re.fullmatch(r"([-+]?)(0[xX][0-9a-fA-F]+|0[bB][01]+|0[oO][0-7]+|0[0-7]*|[1-9][0-9]*)", s)
    if not m:
        return False
    sign, body = m.groups()
    low = body.lower()
    if low.startswith(("0x", "0b", "0o")):
        v = int(body[2:], {"x": 16, "b": 2, "o": 8}[low[1]])
    elif body[0] == "0" and len(body) > 1:
        v = int(body[1:], 8)
    else:
        v = int(body)
    if sign == "-":
        return -v >= _INT64[0]
    return v <= 2**64 - 1


def _go_parse_float(s: str) -> bool:
    """strconv.ParseFloat(s, 64) accepts s (no underscores, no range errors that matter here)."""
    low = s.lower().lstrip("+-")
    if low in ("inf", "infinity", "nan") and not (low == "nan" and s[:1] in "+-"):
        return True
    return bool(_GO_DEC_FLOAT.match(s) or _GO_HEX_FLOAT.match(s)) and _finite(s)


def _finite(s: str) -> bool:
    try:
        return math.isfinite(float.fromhex(s) if "x" in s.lower() else float(s))
    except (ValueError, OverflowError):
        return False


def _valid_ts(m: re.Match[str]) -> bool:
    import calendar

    y, mo, d = int(m.group(1)), int(m.group(2)), int(m.group(3))
    if not (1 <= mo <= 12 and 1 <= d <= calendar.monthrange(y or 2000, mo)[1]):
        return False
    hms = [g for g in m.groups()[3:] if g is not None]
    if hms:
        h, mi, sec = map(int, hms)
        return h < 24 and mi < 60 and sec < 60
    return True


def _is_timestamp(s: str) -> bool:
    m = _TS_DATE.match(s) or _TS_FULL.match(s)
    return bool(m) and _valid_ts(m)  # type: ignore[arg-type]


def _resolves_to_str(s: str) -> bool:
    """yaml.v3 ``resolve("", s)`` returns !!str."""
    if s == "":
        return False
    c = s[0]
    if c in "yYnNtTfFoO~":
        return s not in _RESOLVE_MAP
    if c == ".":
        return s not in _RESOLVE_MAP and not (_GO_DEC_FLOAT.match(s) and _finite(s))
    if c in "+-0123456789":
        if s in _RESOLVE_MAP:
            return False
        if _is_timestamp(s):
            return False
        plain = s.replace("_", "")
        if _go_parse_int(plain):
            return False
        if _YAML_FLOAT.match(plain) and _go_parse_float(plain):
            return False
        for pfx, base in (("0b", 2), ("-0b", 2), ("0o", 8), ("-0o", 8)):
            if plain.startswith(pfx):
                digits = plain[len(pfx) :]
                if digits and all(ch in "01234567"[:base] for ch in digits):
                    return False
                break
        return True
    return True


def _can_use_plain(s: str) -> bool:
    return _resolves_to_str(s) and not (_is_base60(s) or s in _OLD_BOOL)


def _is_base60(s: str) -> bool:
    return bool(s) and (s[0] in "+-" or s[0].isdigit()) and ":" in s and bool(_BASE60.match(s))


def go_format_float(f: float) -> str:
    """strconv.FormatFloat(f, 'g', -1, 64) with yaml.v3's inf/nan spellings."""
    if math.isnan(f):
        return ".nan"
    if math.isinf(f):
        return ".inf" if f > 0 else "-.inf"
    neg = f < 0 or (f == 0 and math.copysign(1, f) < 0)
    t = Decimal(repr(abs(f))).normalize().as_tuple()
    digits = "".join(map(str, t.digits)).rstrip("0") or "0"
    exp_ = int(t.exponent)  # type: ignore[arg-type]
    dp = len(t.digits) + exp_ if digits != "0" else 1
    x = dp - 1
    if x < -4 or x >= 6 and not (f == 0):
        mant = digits[0] + ("." + digits[1:] if len(digits) > 1 else "")
        out = f"{mant}e{'-' if x < 0 else '+'}{abs(x):02d}"
    elif digits == "0":
        out = "0"
    elif dp <= 0:
        out = "0." + "0" * -dp + digits
    elif dp >= len(digits):
        out = digits + "0" * (dp - len(digits))
    else:
        out = digits[:dp] + "." + digits[dp:]
    return ("-" if neg else "") + out


def _number_text(v: int | float) -> str:
    """Go: json.Marshal then json.Number.Int64(), else Float64() → FormatFloat 'g'."""
    if isinstance(v, int):
        if _INT64[0] <= v <= _INT64[1]:
            return str(v)
        return go_format_float(float(v))
    if v.is_integer() and abs(v) < 1e21 and _INT64[0] <= v <= _INT64[1]:
        return str(int(v))
    return go_format_float(v)


# --- scalar analysis (libyaml yaml_emitter_analyze_scalar) ----------------------------

PLAIN, SINGLE, DOUBLE, LITERAL = "plain", "single", "double", "literal"
_BREAKS = "\r\n\x85  "


def _printable(c: str) -> bool:
    o = ord(c)
    return o == 0x0A or 0x20 <= o <= 0x7E or 0xA0 <= o <= 0xD7FF or (0xE000 <= o <= 0xFFFD and o != 0xFEFF)


class _Analysis:
    __slots__ = ("multiline", "block_plain", "single_ok", "block_ok")

    def __init__(self, s: str):
        if s == "":
            self.multiline, self.block_plain, self.single_ok, self.block_ok = False, True, True, False
            return
        block_ind = line_breaks = special = tabs = False
        lead_space = lead_break = trail_space = trail_break = break_space = space_break = False
        prev_space = prev_break = False
        if s.startswith(("---", "...")):
            block_ind = True
        preceded_ws = True
        n = len(s)
        for i, c in enumerate(s):
            followed_ws = i + 1 >= n or s[i + 1] in " \t"
            # only block context is emitted, so flow indicators are not tracked
            if i == 0:
                if c in "#,[]{}&*!|>'\"%@`" or (c in "?:-" and followed_ws):
                    block_ind = True
            elif (c == ":" and followed_ws) or (c == "#" and preceded_ws):
                block_ind = True
            if c == "\t":
                tabs = True
            elif not _printable(c):
                special = True
            if c == " ":
                lead_space = lead_space or i == 0
                trail_space = trail_space or i == n - 1
                break_space = break_space or prev_break
                prev_space, prev_break = True, False
            elif c in _BREAKS:
                line_breaks = True
                lead_break = lead_break or i == 0
                trail_break = trail_break or i == n - 1
                space_break = space_break or prev_space
                prev_space, prev_break = False, True
            else:
                prev_space = prev_break = False
            preceded_ws = c in " \t" or c in _BREAKS
        self.multiline = line_breaks
        self.block_plain = self.single_ok = self.block_ok = True
        if lead_space or lead_break or trail_space or trail_break:
            self.block_plain = False
        if trail_space:
            self.block_ok = False
        if break_space:
            self.block_plain = self.single_ok = False
        if space_break or tabs or special:
            self.block_plain = self.single_ok = False
        if space_break or special:
            self.block_ok = False
        if line_breaks or block_ind:
            self.block_plain = False


def _select_style(s: str, requested: str, simple_key: bool) -> tuple[str, _Analysis]:
    a = _Analysis(s)
    style = requested
    if simple_key and a.multiline:
        style = DOUBLE
    if style == PLAIN:
        if not a.block_plain or (s == "" and simple_key):
            style = SINGLE
    if style == SINGLE and not a.single_ok:
        style = DOUBLE
    if style == LITERAL and (not a.block_ok or simple_key):
        style = DOUBLE
    return style, a


_ESCAPES = {
    0x00: "0", 0x07: "a", 0x08: "b", 0x09: "t", 0x0A: "n", 0x0B: "v", 0x0C: "f", 0x0D: "r", 0x1B: "e",
    0x22: '"', 0x5C: "\\", 0x85: "N", 0xA0: "_", 0x2028: "L", 0x2029: "P",
}  # fmt: skip


def _double_quoted(s: str) -> str:
    bom = s.startswith("﻿")
    out = ['"']
    for c in s:
        o = ord(c)
        if not _printable(c) or bom or c in _BREAKS or c in '"\\':
            esc = _ESCAPES.get(o)
            if esc is None:
                esc = f"x{o:02X}" if o <= 0xFF else (f"u{o:04X}" if o <= 0xFFFF else f"U{o:08X}")
            out.append("\\" + esc)
        else:
            out.append(c)
    out.append('"')
    return "".join(out)


def _stringv_style(s: str) -> str:
    """The style yaml.v3's encoder requests for a plain Go string."""
    if "\n" in s:
        return LITERAL
    return PLAIN if _can_use_plain(s) else DOUBLE


def _node_round_trip(s: str) -> tuple[str, str]:
    """yaml.v3 renders each scalar node by encoding the string on its own and parsing it
    back (``Node.Encode``). With line breaks in play that trip is lossy (the literal
    writer drops a leading break; single quotes fold around LS/PS), so the value and
    style that reach the document can differ from ``s``; reproduce that."""
    import yaml

    first = _Emitter()
    first.scalar(s, in_seq=False, simple_key=False, requested=_stringv_style(s))
    first.write_indent()
    text = "".join(first.out)
    style = {"|": LITERAL, '"': DOUBLE, "'": SINGLE}.get(text[:1], PLAIN)
    if style == LITERAL and s[0] == "\t":
        # libyaml cannot auto-detect the indentation of a block starting with a tab
        raise ValueError("yaml: line 2: found a tab character where an indentation space is expected")
    return yaml.load(text, Loader=yaml.BaseLoader), style


# --- the emitter ---------------------------------------------------------------------


class _Emitter:
    """Just enough of libyaml's writer state (column, whitespace, indention) to lay out
    block mappings, block sequences and scalars exactly as yaml.v3 does."""

    def __init__(self) -> None:
        self.out: list[str] = []
        self.column = 0
        self.whitespace = True
        self.indention = True
        self.indent = -1
        self.indents: list[int] = []

    # primitives
    def put(self, s: str) -> None:
        self.out.append(s)
        self.column += len(s)

    def put_break(self) -> None:
        self.out.append("\n")
        self.column = 0
        self.indention = True

    def write_indent(self) -> None:
        indent = max(self.indent, 0)
        if not self.indention or self.column > indent or (self.column == indent and not self.whitespace):
            self.put_break()
        if self.column < indent:
            self.put(" " * (indent - self.column))
        self.whitespace = True

    def indicator(self, s: str, need_ws: bool, is_ws: bool, is_indention: bool) -> None:
        if need_ws and not self.whitespace:
            self.put(" ")
        self.put(s)
        self.whitespace = is_ws
        self.indention = self.indention and is_indention

    def increase_indent(self, flow: bool, in_sequence_item: bool) -> None:
        self.indents.append(self.indent)
        if self.indent < 0:
            self.indent = INDENT if flow else 0
        elif in_sequence_item:
            self.indent += 2
        else:
            self.indent = INDENT * ((self.indent + INDENT) // INDENT)

    def pop_indent(self) -> None:
        self.indent = self.indents.pop()

    # nodes
    def node(self, v: Any, *, root: bool = False, in_seq: bool = False, simple_key: bool = False) -> None:
        if isinstance(v, dict):
            if not v:
                self.indicator("{", True, True, False)
                self.indicator("}", False, False, False)
                return
            self.increase_indent(False, in_seq)
            for k in sorted(v, key=_key_sort):
                self.write_indent()
                self.node(k, simple_key=True)
                self.indicator(":", False, False, False)
                self.node(v[k])
            self.pop_indent()
        elif isinstance(v, list):
            if not v:
                self.indicator("[", True, True, False)
                self.indicator("]", False, False, False)
                return
            self.increase_indent(False, in_seq)
            for item in v:
                self.write_indent()
                self.indicator("-", True, False, True)
                self.node(item, in_seq=True)
            self.pop_indent()
        else:
            self.scalar(v, in_seq=in_seq, simple_key=simple_key)

    def scalar(self, v: Any, *, in_seq: bool, simple_key: bool, requested: str | None = None) -> None:
        if requested is not None:
            text = v
        elif v is None:
            text, requested = "null", PLAIN
        elif v is True or v is False:
            text, requested = ("true" if v else "false"), PLAIN
        elif isinstance(v, (int, float)):
            text, requested = _number_text(v), PLAIN
        elif v == "<<":
            self.increase_indent(True, in_seq)  # yaml.v3 tags the merge key explicitly
            self.indicator("!!merge", True, False, False)
            self.put(" <<")
            self.whitespace = self.indention = False
            self.pop_indent()
            return
        elif isinstance(v, str):
            if any(c in _BREAKS for c in v):
                text, requested = _node_round_trip(v)
            else:
                text, requested = v, _stringv_style(v)
        else:
            raise TypeError(f"cannot render {type(v).__name__} as YAML")
        style, _ = _select_style(text, requested, simple_key)
        if simple_key and len(text) > 128:
            raise ValueError("mapping key too long for a simple key")
        self.increase_indent(True, in_seq)
        if style == PLAIN:
            if text and not self.whitespace:
                self.put(" ")
            self.put(text)
            if text:
                self.whitespace = False
            self.indention = False
        elif style == SINGLE:
            self.indicator("'", True, False, False)
            self.single_quoted_body(text)
            self.indicator("'", False, False, False)
            self.whitespace = self.indention = False
        elif style == DOUBLE:
            self.indicator('"', True, False, False)
            body = _double_quoted(text)[1:-1]
            self.put(body)
            self.indicator('"', False, False, False)
            self.whitespace = self.indention = False
        else:
            self.literal(text)
        self.pop_indent()

    def write_break(self, c: str) -> None:
        if c == "\n":
            self.put_break()
        else:
            self.out.append(c)
            self.column = 0
            self.indention = True

    def single_quoted_body(self, s: str) -> None:
        breaks = False
        for c in s:
            if c in _BREAKS:
                if not breaks and c == "\n":
                    self.put_break()
                self.write_break(c)
                breaks = True
            else:
                if breaks:
                    self.write_indent()
                self.put("''" if c == "'" else c)
                if c != " ":
                    self.indention = False
                breaks = False

    def literal(self, s: str) -> None:
        self.indicator("|", True, False, False)
        if s and (s[0] == " " or s[0] in _BREAKS):
            self.indicator(str(INDENT), False, False, False)
        if not s or s[-1] not in _BREAKS:
            chomp = "-"
        elif len(s) == 1 or s[-2] in _BREAKS:
            chomp = "+"
        else:
            chomp = ""
        if chomp:
            self.indicator(chomp, False, False, False)
        self.whitespace = True
        breaks = True
        for c in s:
            if c in _BREAKS:
                self.write_break(c)
                breaks = True
            else:
                if breaks:
                    self.write_indent()
                self.put(c)
                self.indention = False
                breaks = False
