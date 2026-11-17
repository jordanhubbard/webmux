"""Verify the retained upstream source and the explicitly recorded local patch."""
import hashlib
import json
from pathlib import Path

root = Path(__file__).resolve().parents[1] / "webmux/server/internal/thirdparty/vt"
manifest = json.loads((root / "WEBMUX_UPSTREAM.json").read_text())
original = manifest["original_files_sha256"]
modified = manifest["webmux_modified_files_sha256"]
assert modified.keys() <= original.keys()
for name, digest in original.items():
    expected = modified.get(name, digest)
    actual = hashlib.sha256((root / name).read_bytes()).hexdigest()
    assert actual == expected, f"Unrecorded vt source change: {name}"
additions = {
    "WEBMUX_UPSTREAM.json", "WEBMUX.md", "xterm_no_wrap.go",
    "xterm_no_wrap_test.go", "testdata/webmux-xterm-nowrap.json",
 "xterm_underline.go", "xterm_underline_test.go",
 "xterm_underline_reference_test.go", "testdata/webmux-xterm-underline.json",
}
actual_files = {p.relative_to(root).as_posix() for p in root.rglob("*") if p.is_file()}
assert actual_files == original.keys() | additions, "Unrecorded vt source addition or removal"
print(f"Verified {len(original)} upstream files; {len(modified)} explicitly patched files")
