#!/usr/bin/env python3
"""Compare the RFC 2119 keywords in docs/specifications/*.md at a base commit with the working tree.

Usage (from anywhere in the repository; `make spec-kwdiff` wraps it):

    python3 scripts/spec-kwdiff.py [--check] <base> [name ...]

<base> is any commit git can resolve, such as origin/main or a SHA. Each name
(wire.md, auth.md, ...) restricts the comparison to that file on both sides.

What it prints, in order:

1. A table of keyword counts per file, at the base and in the working tree,
   one column each for MUST, MUST NOT, SHOULD, SHOULD NOT and MAY, and OTHER
   for SHALL, SHALL NOT, REQUIRED, RECOMMENDED and OPTIONAL. A changed count
   reads "base->head", and the last column marks a file whose counts changed,
   or that exists on one side only (added or removed). A total line follows.
2. Each base sentence with a keyword that no longer appears word for word in
   the same files of the working tree, with the closest working-tree sentence
   and its keywords. The closest sentence is looked up in every spec file, so
   a sentence moved to another file shows where it went. A working-tree
   sentence that ends a line in a colon is also tried joined with the list
   that follows it, so a sentence split into a list pairs with the whole
   list; that pairing points at the colon's sentence and ends "with its
   list". Each one is marked:
     KEYWORDS CHANGED  the closest sentence has other keywords: a demotion,
                       a promotion, a dropped or an added rule;
     NO CLOSE MATCH    nothing in the working tree is close: a deleted rule
                       or a rewrite too large to pair;
     same keywords     a reword, a reflow or a split into a list.

How it reads the Markdown: lines of one paragraph or list item are joined
before the text is split into sentences, so rewrapping a paragraph changes
nothing; each table cell is read on its own; fenced code blocks are skipped;
link targets, emphasis and whitespace are ignored when comparing; and a
keyword inside `inline code` is a mention, not a rule, so it is not counted.

It is a report: it exits 0 whatever it finds. With --check it exits 1 when
any file's keyword counts differ between the base and the working tree. A
base that is missing or unknown, or a name that matches no file, exits 2
with a message naming the fix. It needs only git and the Python 3.9
standard library.
"""

import argparse
import collections
import difflib
import pathlib
import re
import subprocess
import sys

SPEC_DIR = "docs/specifications"

# Longest alternative first, so "MUST NOT" is one keyword, not MUST.
KEYWORD = re.compile(
    r"\b(MUST NOT|MUST|SHALL NOT|SHALL|SHOULD NOT|SHOULD|REQUIRED|RECOMMENDED|MAY|OPTIONAL)\b"
)
COLUMNS = ["MUST", "MUST NOT", "SHOULD", "SHOULD NOT", "MAY", "OTHER"]
LINK = re.compile(r"\[([^\]]*)\]\([^)]*\)")
CODE_SPAN = re.compile(r"`[^`]*`")
FENCE = re.compile(r"^(```+|~~~+)")
HEADING = re.compile(r"^#{1,6}\s")
LIST_ITEM = re.compile(r"^([-*+]|\d+[.)])\s+")
SENTENCE_BREAK = re.compile(r"(?<=[.!?])\s+(?=[A-Z*`\[(])")
WORD = re.compile(r"[A-Za-z_][A-Za-z0-9_.]{5,}")
SHOW = 400  # characters of a sentence printed


def fail(message):
    print("spec-kwdiff: " + message, file=sys.stderr)
    sys.exit(2)


def git(root, *args):
    """Run git in root; return (exit code, stdout as text)."""
    done = subprocess.run(["git", "-C", str(root)] + list(args), capture_output=True)
    return done.returncode, done.stdout.decode("utf-8", "replace")


def normalise(text):
    text = LINK.sub(r"\1", text)
    text = text.replace("*", "")
    return re.sub(r"\s+", " ", text).strip()


def keywords(sentence):
    """The keywords a sentence uses, leaving out those inside code spans."""
    return KEYWORD.findall(CODE_SPAN.sub("", sentence))


def blocks(text):
    """Yield (first line number, text) for each paragraph, list item, heading and table cell."""
    lines = []
    first = 0
    in_fence = False
    for number, raw in enumerate(text.split("\n"), 1):
        line = raw.strip()
        if FENCE.match(line):
            if lines:
                yield first, "\n".join(lines)
                lines = []
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        while line.startswith(">"):
            line = line[1:].strip()
        if not line or line.startswith("|") or HEADING.match(line) or LIST_ITEM.match(line):
            if lines:
                yield first, "\n".join(lines)
                lines = []
        if not line:
            continue
        if line.startswith("|"):
            for cell in line.strip("|").split("|"):
                yield number, cell
            continue
        if HEADING.match(line):
            yield number, line.lstrip("#")
            continue
        if not lines:
            first = number
        lines.append(LIST_ITEM.sub("", line, count=1))
    if lines:
        yield first, "\n".join(lines)


def split(first, block):
    """Return (line number, normalised sentence) for every sentence of one block."""
    out = []
    start = 0
    breaks = [m.span() for m in SENTENCE_BREAK.finditer(block)] + [(len(block), len(block))]
    for end, after in breaks:
        sentence = normalise(block[start:end])
        if sentence:
            out.append((first + block.count("\n", 0, start), sentence))
        start = after
    return out


def sentences(text):
    """Return (line number, normalised sentence) for every sentence of a Markdown text."""
    out = []
    for first, block in blocks(text):
        out.extend(split(first, block))
    return out


def indent(line):
    return len(line) - len(line.lstrip())


def list_after(lines, last):
    """Return the lines of the list that follows line number last, item markers removed, or [] when none follows.

    The list runs over its items and their indented continuation lines, and
    over a blank line only when an item follows it. An item must be indented
    at least as deep as line last, and deeper when line last is itself an
    item, so a sibling of a nested list's parent ends the nested list.
    """
    stem = lines[last - 1]
    floor = indent(stem) + (1 if LIST_ITEM.match(stem.strip()) else 0)
    out = []
    item_indent = None
    i = last
    while i < len(lines):
        line = lines[i].strip()
        if FENCE.match(line):
            break
        if not line:
            k = i + 1
            while k < len(lines) and not lines[k].strip():
                k += 1
            if k < len(lines) and LIST_ITEM.match(lines[k].strip()) and indent(lines[k]) >= floor:
                i = k
                continue
            break
        if LIST_ITEM.match(line):
            if indent(lines[i]) < floor:
                break
            item_indent = indent(lines[i])
            out.append(LIST_ITEM.sub("", line, count=1))
        elif item_indent is not None and indent(lines[i]) > item_indent:
            out.append(line)
        else:
            break
        i += 1
    return out


def list_splits(text):
    """Return (line number, normalised text) for each sentence that ends a line in a colon and a list follows.

    The text is the sentence joined with the whole list, as one sentence; the
    line number is the sentence's own.
    """
    lines = text.split("\n")
    out = []
    for first, block in blocks(text):
        last = first + block.count("\n")
        line = lines[last - 1].strip()
        if not normalise(block).endswith(":") or line.startswith("|") or HEADING.match(line):
            continue
        items = list_after(lines, last)
        stem = split(first, block)
        if items and stem:
            number, sentence = stem[-1]
            out.append((number, normalise(sentence + " " + " ".join(items))))
    return out


def counts(sentence_list):
    tally = collections.Counter()
    for _, sentence in sentence_list:
        for word in keywords(sentence):
            tally[word if word in COLUMNS else "OTHER"] += 1
    return tally


def cell(base, head):
    return str(base) if base == head else "{}->{}".format(base, head)


def print_table(names, base_counts, head_counts):
    rows = [["file"] + COLUMNS + [""]]
    total_base, total_head = collections.Counter(), collections.Counter()
    for name in names:
        b = base_counts.get(name)
        h = head_counts.get(name)
        bc, hc = b or collections.Counter(), h or collections.Counter()
        total_base.update(bc)
        total_head.update(hc)
        if b is None:
            mark = "added"
        elif h is None:
            mark = "removed"
        else:
            mark = "changed" if any(bc[c] != hc[c] for c in COLUMNS) else ""
        rows.append([name] + [cell(bc[c], hc[c]) for c in COLUMNS] + [mark])
    changed = any(total_base[c] != total_head[c] for c in COLUMNS)
    rows.append(["total"] + [cell(total_base[c], total_head[c]) for c in COLUMNS] + ["changed" if changed else ""])
    widths = [max(len(row[i]) for row in rows) for i in range(len(rows[0]))]
    for row in rows:
        print("  ".join(text.ljust(width) for text, width in zip(row, widths)).rstrip())


def main():
    parser = argparse.ArgumentParser(
        prog="spec-kwdiff",
        description="Compare the RFC 2119 keywords in the specifications at a base commit with the working tree.",
    )
    parser.add_argument("--check", action="store_true", help="exit 1 when any file's keyword counts change")
    parser.add_argument("base", nargs="?", default="", help="the commit to compare with, such as origin/main")
    parser.add_argument("names", nargs="*", help="spec file names to compare, such as wire.md (default: all)")
    args = parser.parse_args()

    root = pathlib.Path(__file__).resolve().parent.parent
    if not args.base:
        fail("no base commit; pass one, for example make spec-kwdiff BASE=origin/main")
    code, sha = git(root, "rev-parse", "--verify", "--quiet", args.base + "^{commit}")
    if code != 0:
        fail(
            "unknown base {!r}: it names no commit here; run git fetch origin, "
            "or pass a branch, tag or SHA that exists, as BASE=<ref>".format(args.base)
        )
    sha = sha.strip()

    code, listing = git(root, "ls-tree", "-z", "--name-only", sha, SPEC_DIR + "/")
    if code != 0:
        fail("git ls-tree failed on {} {}/".format(sha[:8], SPEC_DIR))
    base_paths = {pathlib.PurePosixPath(p).name: p for p in listing.split("\0") if p.endswith(".md")}
    head_paths = {p.name: p for p in sorted((root / SPEC_DIR).glob("*.md"))}

    wanted = sorted(set(pathlib.PurePosixPath(n).name for n in args.names))
    unknown = [n for n in wanted if n not in base_paths and n not in head_paths]
    if unknown:
        fail(
            "no file named {} in {}/ at the base or in the working tree; "
            "FILES takes names such as wire.md".format(", ".join(unknown), SPEC_DIR)
        )
    names = wanted or sorted(set(base_paths) | set(head_paths))

    head_all = {}  # every head file, for the closest-sentence lookup
    head_lists = {}  # every head sentence joined with the list it introduces
    for name, path in head_paths.items():
        text = path.read_text(encoding="utf-8")
        head_all[name] = sentences(text)
        head_lists[name] = list_splits(text)
    base_all = {}
    for name in names:
        if name in base_paths:
            code, text = git(root, "show", "{}:{}".format(sha, base_paths[name]))
            if code != 0:
                fail("git show failed on {}:{}".format(sha[:8], base_paths[name]))
            base_all[name] = sentences(text)

    base_counts = {name: counts(base_all[name]) for name in names if name in base_all}
    head_counts = {name: counts(head_all[name]) for name in names if name in head_all}

    label = args.base if sha.startswith(args.base) else "{} ({})".format(args.base, sha[:8])
    print("RFC 2119 keywords in {}/: base {} against the working tree".format(SPEC_DIR, label))
    print()
    print_table(names, base_counts, head_counts)

    # Base keyword sentences no longer present word for word in the same files.
    head_kept = {s for name in names for _, s in head_all.get(name, []) if keywords(s)}
    gone = [
        (name, line, s)
        for name in names
        for line, s in base_all.get(name, [])
        if keywords(s) and s not in head_kept
    ]
    pool = [(name, line, s) for name in sorted(head_all) for line, s in head_all[name]]
    # A sentence split into a list also has its introducing sentence joined
    # with the whole list as a candidate, so it is not paired with whichever
    # item is closest. These candidates come after every single sentence and
    # are ranked apart, so the single sentences tried stay the same.
    singles = len(pool)
    pool += [(name, line, s) for name in sorted(head_lists) for line, s in head_lists[name]]
    joined = {s for _, _, s in pool[singles:]}
    where = {}
    index = collections.defaultdict(list)
    for i, (name, line, s) in enumerate(pool):
        where.setdefault(s, (name, line))
        for word in set(WORD.findall(s)):
            index[word].append(i)

    groups = {"KEYWORDS CHANGED": [], "NO CLOSE MATCH": [], "same keywords": []}
    for name, line, s in gone:
        votes = collections.Counter()
        for word in set(WORD.findall(s)):
            votes.update(index.get(word, [])[:2000])
        # Most shared words first; on a tie, the base sentence's own file,
        # then file order. Sets iterate in a different order in each run, so
        # without a full order the pick would change from run to run.
        ranked = sorted(votes, key=lambda i: (-votes[i], pool[i][0] != name, i))
        candidates = [pool[i][2] for i in ranked if i < singles][:60]
        candidates += [pool[i][2] for i in ranked if i >= singles][:20]
        best = difflib.get_close_matches(s, candidates, n=1, cutoff=0.45)
        if not best:
            groups["NO CLOSE MATCH"].append((name, line, s, None))
            continue
        same = collections.Counter(keywords(s)) == collections.Counter(keywords(best[0]))
        groups["same keywords" if same else "KEYWORDS CHANGED"].append((name, line, s, best[0]))

    print()
    print(
        "Base keyword sentences not word for word in the working tree: {} "
        "({} keywords changed, {} no close match, {} same keywords)".format(
            len(gone), len(groups["KEYWORDS CHANGED"]), len(groups["NO CLOSE MATCH"]), len(groups["same keywords"])
        )
    )
    for mark, items in groups.items():
        for name, line, s, best in items:
            base_kw = ",".join(keywords(s))
            if best is None:
                print("\n- {}  {}:{} [{}]".format(mark, name, line, base_kw))
            else:
                head_name, head_line = where[best]
                head_kw = ",".join(keywords(best)) or "none"
                print(
                    "\n- {}  {}:{} [{}] -> {}:{} [{}]{}".format(
                        mark, name, line, base_kw, head_name, head_line, head_kw, " with its list" if best in joined else ""
                    )
                )
            print("    base: " + s[:SHOW])
            if best is not None:
                print("    head: " + best[:SHOW])

    changed = [n for n in names if any(base_counts.get(n, {}).get(c, 0) != head_counts.get(n, {}).get(c, 0) for c in COLUMNS)]
    print()
    if changed:
        print("spec-kwdiff: keyword counts changed in {} file(s): {}".format(len(changed), ", ".join(changed)))
    else:
        print("spec-kwdiff: keyword counts unchanged in {} file(s)".format(len(names)))
    if args.check and changed:
        sys.exit(1)


if __name__ == "__main__":
    main()
