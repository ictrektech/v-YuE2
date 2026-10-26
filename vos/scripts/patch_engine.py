#!/usr/bin/env python3
"""Bound native GGML thread count with a fixed, auditable VOS setting."""
from pathlib import Path
import sys
p=Path(sys.argv[1])/ 'src/backend.h'
s=p.read_text()
old='    int n = (int) std::thread::hardware_concurrency() / 2;'
new='''    // VOS resource calibration pins this setting; desktop behaviour is unchanged.
    const char * configured = std::getenv("YUE_CPU_THREADS");
    if (configured) {
        int requested = std::atoi(configured);
        if (requested >= 1 && requested <= 128) return requested;
    }
'''+old
if 'std::getenv("YUE_CPU_THREADS")' not in s:
    if s.count(old)!=1: raise SystemExit('Pinned engine source mismatch')
    p.write_text(s.replace(old,new))
