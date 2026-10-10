#!/usr/bin/env python3
"""Print R2 version prefixes to delete, keeping the newest releases.

Root objects such as latest.json are ignored. A prefix must look like 0.2.46/.
"""

import re
import sys

PREFIX = re.compile(r"^v?(\d+)\.(\d+)\.(\d+)/?$")


def drop_prefixes(lines, keep):
    found = {}
    for line in lines:
        for token in line.split():
            name = token.strip().strip("/")
            match = PREFIX.match(name)
            if not match:
                continue
            found[tuple(int(part) for part in match.groups())] = name + "/"
    ordered = [found[key] for key in sorted(found)]
    if keep < 0:
        keep = 0
    if len(ordered) <= keep:
        return []
    return ordered[:-keep] if keep else ordered


def main(argv):
    keep = 3
    if len(argv) > 1:
        keep = int(argv[1])
    for prefix in drop_prefixes(sys.stdin, keep):
        print(prefix)


if __name__ == "__main__":
    main(sys.argv)
