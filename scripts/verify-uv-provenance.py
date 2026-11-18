"""Verify the retained upstream source and the explicitly recorded local patch."""
import hashlib
import json
from pathlib import Path

root = Path(__file__).resolve().parents[1] / "webmux/server/internal/thirdparty/ultraviolet"
manifest = json.loads((root / "WEBMUX_UPSTREAM.json").read_text())
original = manifest["original_files_sha256"]
modified = manifest["webmux_modified_files_sha256"]
assert modified.keys() <= original.keys()
for name, digest in original.items():
    expected = modified.get(name, digest)
    actual = hashlib.sha256((root / name).read_bytes()).hexdigest()
    assert actual == expected, f"Unrecorded ultraviolet source change: {name}"
additions = {
 "WEBMUX_UPSTREAM.json", "WEBMUX.md", "underline_flags.go", "underline_flags_test.go",
}
actual_files = {p.relative_to(root).as_posix() for p in root.rglob("*") if p.is_file()}
assert actual_files == original.keys() | additions, "Unrecorded ultraviolet source addition or removal"
print(f"Verified {len(original)} ultraviolet upstream files; {len(modified)} explicitly patched files")
