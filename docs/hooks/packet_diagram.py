"""MkDocs hook: packet diagram custom fence for pymdownx.superfences.

Registers a ``packet`` code fence that renders frame format diagrams
using a vertical (stacked) layout.

DSL syntax (one field per line)::

    field_name | size_bytes | color_category

* Size ``0`` = variable length (rendered with a stripe pattern).
* Color categories: ethernet, ethertype, vlan, protocol, payload, default.
* Lines starting with ``#`` are comments.

Bracket annotations (after a ``---`` separator)::

    ---
    [first_field : last_field] Label Text

* Each ``---`` starts a new bracket column (innermost first).
* Field names must match exactly (case-sensitive).
* Multiple brackets in the same section share a column (non-overlapping).
"""

import html as _html
import re as _re


def fence_packet_format(source, language, class_name, options, md, **kwargs):
    """Parse the DSL and return a vertical CSS-grid HTML packet diagram."""
    # Split into sections on '---' lines.
    sections = []
    current = []
    for line in source.strip().splitlines():
        if line.strip() == "---":
            sections.append(current)
            current = []
        else:
            current.append(line)
    sections.append(current)

    fields = []
    for line in sections[0]:
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        parts = [p.strip() for p in line.split("|")]
        if len(parts) < 2:
            continue
        name = parts[0]
        try:
            size = int(parts[1])
        except ValueError:
            continue
        category = parts[2] if len(parts) > 2 else "default"
        fields.append((name, size, category))

    if not fields:
        return ""

    field_names = [f[0] for f in fields]
    num_fields = len(fields)

    field_elements = []
    for name, size, category in fields:
        escaped_name = _html.escape(name)
        modifier = f"packet-field--{category}"
        variable_cls = " packet-field--variable" if size == 0 else ""
        size_label = (
            "Variable"
            if size == 0
            else f"{size} {'byte' if size == 1 else 'bytes'}"
        )

        field_elements.append(
            f'<div class="packet-v-row {modifier}{variable_cls}">'
            f'<span class="packet-v-row__label">{escaped_name}</span>'
            f'<span class="packet-v-row__size">{size_label}</span>'
            f"</div>"
        )

    bracket_html = []
    bracket_col = 2
    for section in sections[1:]:
        col_has_bracket = False
        for line in section:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            m = _re.match(r"\[(.+?)\s*:\s*(.+?)\]\s+(.+)", line)
            if not m:
                continue
            first = m.group(1).strip()
            last = m.group(2).strip()
            label = m.group(3).strip()
            try:
                start = field_names.index(first)
                end = field_names.index(last)
            except ValueError:
                continue
            if start > end:
                start, end = end, start
            escaped = _html.escape(label)
            row_start = start + 1
            row_end = end + 2
            bracket_html.append(
                f'<div class="packet-v-bracket"'
                f" style=\"grid-row: {row_start} / {row_end};"
                f' grid-column: {bracket_col};">'
                f'<div class="packet-v-bracket__line"></div>'
                f'<div class="packet-v-bracket__label">{escaped}</div>'
                f"</div>"
            )
            col_has_bracket = True
        if col_has_bracket:
            bracket_col += 1

    num_bracket_cols = bracket_col - 2
    col_template = "1fr" + " auto" * num_bracket_cols

    return (
        f'<div class="packet-diagram--vertical"'
        f" style=\"grid-template-columns: {col_template};"
        f' grid-template-rows: repeat({num_fields}, auto);">'
        f'<div class="packet-v-fields"'
        f' style="grid-row: 1 / {num_fields + 1};">'
        f'{"".join(field_elements)}'
        f"</div>"
        f'{"".join(bracket_html)}'
        f"</div>"
    )


def on_config(config):
    """Inject the packet fence into pymdownx.superfences custom_fences."""
    sf_config = config.mdx_configs.setdefault("pymdownx.superfences", {})
    fences = sf_config.setdefault("custom_fences", [])
    fences.append(
        {
            "name": "packet",
            "class": "packet",
            "format": fence_packet_format,
        }
    )
    return config
