#!/usr/bin/env python3
"""STAGE-148: generate ordinary exits; never changes the legacy landing table."""
from collections import defaultdict, Counter
from pathlib import Path
import hashlib
import json
import struct
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent
MAPS = ROOT / 'original-data/gmsv/data/map'
OUT = ROOT / 'server/gamedata/mapexits_gen.go'


def read_maps(folder):
    maps = {}
    for path in sorted(folder.rglob('*')):
        if not path.is_file():
            continue
        with path.open('rb') as f:
            head = f.read(44)
        if len(head) == 44 and head[:6] == b'LS2MAP':
            floor = struct.unpack('>H', head[6:8])[0]
            maps.setdefault(floor, struct.unpack('>HH', head[40:44]))
    return maps


def near_same_map(dests):
    """True when all landings are on one map and pairwise Manhattan distance <= 1 (STAGE-261)."""
    if len({floor for floor, _, _ in dests}) != 1:
        return False
    points = [(x, y) for _, x, y in dests]
    return all(abs(ax - bx) + abs(ay - by) <= 1
               for i, (ax, ay) in enumerate(points) for bx, by in points[i+1:])


def generate(lines, maps):
    """Return stable components and source-locatable diagnostics (1-based lines)."""
    records, seen, diagnostics = [], set(), []
    for line_no, raw in enumerate(lines, 1):
        text = raw.strip()
        if not text or text.startswith('#'):
            continue
        fields = text.split(':')
        try:
            if len(fields) != 5:
                raise ValueError('format')
            kind, gate, src, dst, params = fields
            if kind != 'NONE' or gate not in ('NULL', 'M', 'A', 'N') or params != 'NULL':
                raise ValueError('unknown condition')
            src, dst = tuple(map(int, src.split(','))), tuple(map(int, dst.split(',')))
            if len(src) != 3 or len(dst) != 3:
                raise ValueError('format')
            for floor, x, y in (src, dst):
                if floor not in maps:
                    raise ValueError(f'missing map {floor}')
                w, h = maps[floor]
                if not (0 <= x < w and 0 <= y < h):
                    raise ValueError(f'out of bounds {floor},{x},{y}')
        except ValueError as error:
            diagnostics.append((line_no, str(error), text))
            continue
        record = (kind, gate, src, dst, params)
        if record in seen:
            diagnostics.append((line_no, 'duplicate', text))
            continue
        seen.add(record)
        records.append((record, line_no))
    # Conflict includes same-map records: an ambiguous trigger must never be executed.
    targets = defaultdict(set)
    for (kind, gate, src, dst, params), _ in records:
        targets[(kind, gate, src, params)].add(dst)
    bad = {key for key, dests in targets.items() if len(dests) > 1}
    # STAGE-261: near-identical landings on one map resolve to the earliest line, as the legacy
    # server does (readmap.c appends, event.c stops at the first matching WARPPOINT).
    resolved = {key for key in bad if near_same_map(targets[key])}
    if resolved:
        first = {}
        for (kind, gate, src, dst, params), line in records:
            key = (kind, gate, src, params)
            if key in resolved and (key not in first or line < first[key]):
                first[key] = line
        kept = []
        for (kind, gate, src, dst, params), line in records:
            key = (kind, gate, src, params)
            if key in resolved and line != first[key]:
                diagnostics.append((line, 'conflict-resolved', f'{src} -> {dst}; kept line {first[key]}'))
            else:
                kept.append(((kind, gate, src, dst, params), line))
        records = kept
        bad -= resolved
    for (kind, gate, src, dst, params), line in records:
        if (kind, gate, src, params) in bad:
            diagnostics.append((line, 'conflict', f'{src} -> {dst}'))
    groups = defaultdict(list)
    for (kind, gate, src, dst, params), _ in records:
        if src[0] != dst[0]:
            groups[(src[0], dst[0], kind, gate, params)].append((src[1:], dst[1:]))
    exits = []
    for (source, target, kind, gate, params), mappings in sorted(groups.items()):
        cells = defaultdict(set)
        for src, dst in mappings:
            cells[src].add(dst)
        todo = set(cells)
        while todo:
            first = min(todo)
            todo.remove(first)
            stack, component = [first], []
            while stack:
                x, y = stack.pop()
                component.append((x, y))
                for neighbor in ((x-1,y), (x+1,y), (x,y-1), (x,y+1)):
                    if neighbor in todo:
                        todo.remove(neighbor)
                        stack.append(neighbor)
            component.sort()
            identity = [source, target, kind, gate, params, *component[0]]
            ident = 'exit-' + hashlib.sha256(json.dumps(identity, separators=(',', ':')).encode()).hexdigest()[:24]
            conflict = any((kind,gate,(source,*src),params) in bad for src in component)
            exits.append(dict(id=ident, source=source, target=target, kind=kind, gate=gate,
                              params=params, conflict=conflict,
                              cells=[(*src,*dst) for src in component for dst in sorted(cells[src])]))
    exits.sort(key=lambda e: (e['source'],e['target'],e['cells'][0][:2],e['gate'],e['id']))
    edges = {(e['source'],e['target']) for e in exits if not e['conflict']}
    for e in exits:
        e['one_way'] = (e['target'],e['source']) not in edges
    return exits, diagnostics


def render(exits):
    lines = ['// Code generated by tools/gen-mapexits.py; DO NOT EDIT.', '', 'package gamedata', '',
             'var mapExits = map[int][]MapExit{']
    last = None
    gates = {'NULL':'all', 'M':'morning', 'A':'noon', 'N':'night'}
    for e in exits:
        if e['source'] != last:
            if last is not None:
                lines.append('\t},')
            last = e['source']
            lines.append(f'\t{last}: {{')
        q = json.dumps
        lines.append(f'\t\t{{ID:{q(e["id"])}, ToFloor:{e["target"]}, Kind:{q(e["kind"])}, '
                     f'TimeGate:{q(gates[e["gate"]])}, Parameters:{q(e["params"])}, '
                     f'Conflict:{str(e["conflict"]).lower()}, OneWay:{str(e["one_way"]).lower()}, Cells: []ExitCell{{')
        for x,y,tx,ty in e['cells']:
            lines.append(f'\t\t\t{{X:{x}, Y:{y}, ToX:{tx}, ToY:{ty}}},')
        lines.append('\t\t}},')
    if last is not None:
        lines.append('\t},')
    lines.append('}')
    return '\n'.join(lines)+'\n'


def main():
    exits, diagnostics = generate((MAPS/'mapwarp.txt').read_text(encoding='gbk').splitlines(), read_maps(MAPS))
    OUT.write_text(render(exits), encoding='utf-8', newline='\n')
    subprocess.run(['gofmt','-w',str(OUT)], check=True)
    for line, reason, text in diagnostics:
        print(f'mapwarp.txt:{line}: {reason}: {text}', file=sys.stderr)
    counts = Counter(reason.split()[0] for _,reason,_ in diagnostics)
    print(f'{len(exits)} exits; diagnostics {dict(counts)}')


if __name__ == '__main__':
    main()
