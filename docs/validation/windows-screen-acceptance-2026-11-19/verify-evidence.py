"""Read retained original uploads; never dispatch tests or alter native state."""
import collections
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path, PurePosixPath
import re
import stat
import struct
import subprocess
import sys
import tempfile
import zipfile

ROOT = Path(sys.argv[6]).resolve() if len(sys.argv) > 6 else Path(__file__).resolve().parent
REPO = Path(sys.argv[1]).resolve()
RAW = Path(sys.argv[2]).resolve()
OLD_ZIP = Path(sys.argv[3]).resolve()
EXTRACTOR = Path(sys.argv[4]).resolve()
GO = sys.argv[5]
FINAL = '0056000940ec0dbc96a010c3b88d349759c51c48'
OLD = '52a8527e7e8a3e0ed0d0af09eadbd9457eae9854'
sha = lambda b: hashlib.sha256(b).hexdigest()
decode = lambda b: json.loads(b.decode('utf-8-sig'))
git = lambda *a: subprocess.check_output(['git', '-C', str(REPO), *a])
events = lambda b: [json.loads(l) for l in b.decode('utf-8-sig').splitlines() if l.strip()]
receipt = {'auditedHead': git('rev-parse', 'HEAD').decode().strip(), 'nativeSource': FINAL,
           'recordedUTC': datetime.now(timezone.utc).isoformat()}


def upload(name):
    meta = json.loads((RAW / f'{name}.metadata.json').read_text())
    manifest = json.loads((RAW / f'{name}.members-sha256.json').read_text())
    data = (RAW / f'{name}.zip').read_bytes()
    assert meta['workflow_run']['head_sha'] == FINAL
    assert meta['digest'] == 'sha256:' + sha(data)
    z = zipfile.ZipFile(RAW / f'{name}.zip')
    assert len(z.namelist()) == len(set(z.namelist())) == len(manifest)
    for entry in z.infolist():
        p = PurePosixPath(entry.filename)
        assert not p.is_absolute() and '..' not in p.parts and '\\' not in entry.filename
        assert not stat.S_ISLNK(entry.external_attr >> 16) and not entry.is_dir()
        assert sha(z.read(entry)) == manifest[entry.filename]
    return z, {'artifact': meta['id'], 'run': meta['workflow_run']['id'],
               'digest': meta['digest'], 'uploadedFiles': len(manifest)}


def screen(name, prefix, machine):
    z, result = upload(name)
    captures = decode(z.read(prefix + '/captures.json'))
    assert len(captures) == len({v['name'] for v in captures}) == 28
    duplicate = next(n for n in z.namelist() if n.endswith('/native-screen-captures.json'))
    assert captures == decode(z.read(duplicate))
    legacy = {}
    for c in captures:
        assert c['starts'] == 1 and c['owner_pid'] > 0 and c['root_pid'] > 0
        assert c['snapshot_profile'] == c['recaptured_profile'] == 'webmux-xterm-state-v2'
        assert c['snapshot_token'] != c['recaptured_token']
        assert all(re.fullmatch('[0-9a-f]{32}', c[k]) for k in ['snapshot_token', 'recaptured_token'])
        if c['legacy_attempted']:
            legacy[c['name']] = c['legacy_refused']
    assert len(legacy) == 6 and sum(legacy.values()) == 5 and not legacy['hidden-1049']
    assert z.read(prefix + '/exit-code.txt').decode('utf-8-sig').strip() == '0'
    tests = events(z.read(prefix + '/tests.jsonl'))
    assert not any(e['Action'] in ['fail', 'skip'] for e in tests)
    assert sum(e['Action'] == 'pass' and bool(e.get('Test')) for e in tests) == 29
    image = z.read(prefix + '/native-screen.test.exe')
    offset = struct.unpack_from('<I', image, 60)[0]
    assert image[offset:offset+4] == b'PE\0\0'
    assert struct.unpack_from('<H', image, offset+4)[0] == machine
    reports = [n for n in z.namelist() if n.endswith('/screen-replay.json')]
    assert len(reports) == 1381
    profiles = collections.Counter()
    for n in reports:
        d = decode(z.read(n))
        assert all(d[k]['equal'] for k in ['initial', 'continued', 'recaptured'])
        assert d['replayReplies'] == [] and not d['referenceUsesSnapshotProfile']
        assert d['initialScratchLinks'] <= 1 and d['recapturedScratchLinks'] <= 1
        profiles[d['referenceProfile']] += 1
        if d['referenceProfile'] == 'bare':
            assert d['referenceMethodsUnchanged']
        for focus in d['focusReports']:
            assert focus['data'] in ['\x1b[I', '\x1b[O']
            assert focus['phase'] in ['snapshot', 'recaptured']
    assert profiles == {'bare': 259, 'shared-hyperlink': 1122}
    decoder = decode(z.read(next(n for n in z.namelist() if n.endswith('/underline-v2-decoder.json'))))
    assert decoder == {'controls': 256, 'failures': [], 'splits': 79, 'cursorSplits': 107,
                       'invalid': 24, 'badEncoding': 4, 'clearedRepeat': True,
                       'reusedRefused': True, 'disarmed': True, 'disposed': True, 'replies': []}
    links = decode(z.read(next(n for n in z.namelist() if n.endswith('/hyperlink-lifecycle.json'))))
    assert links == {'before': {'stock': None, 'fixed': 'https://example.test/alt'},
                     'continued': 'https://example.test/alt', 'resetSize': 0, 'maxHeld': 1,
                     'maxClosed': 0, 'directResetSize': 0,
                     'scrolled': 'https://example.test/scroll', 'disposedSize': 0}
    result.update(captures=28, legacyProbes=legacy, comparisonReports=1381,
                  referenceProfiles=dict(profiles), snapshotReferenceAdapter=False,
                  allThreeStagesEqual=True, replayReplies=0, nativeFailSkip=0,
                  capturePEMachine=hex(machine), captureImageSHA256=sha(image),
                  decoder=decoder, hyperlinkLifecycle=links)
    return z, result


amd, receipt['amd64Screen'] = screen('native-screen-interoperability', '_temp/webmux-native-screen', 0x8664)
arm, receipt['arm64Screen'] = screen('windows-arm64-native-screen', '_temp/webmux-arm64-native/captures', 0xaa64)
arch = decode(arm.read('_temp/webmux-arm64-native/capture-architecture.json'))
assert arch['images']['capture']['sha256'] == receipt['arm64Screen']['captureImageSHA256']
assert arch['nodeArchitecture'] == arch['go']['GOARCH'] == arch['go']['GOHOSTARCH'] == 'arm64'
assert arch['images']['browser']['machine'] == 'amd64' and not arch['rendererNativeARM64']
receipt['arm64Screen']['browserArchitecture'] = 'amd64 emulated; no native ARM64 browser claim'
ordinary = events(arm.read('_temp/webmux-arm64-native/server-tests.jsonl'))
assert not any(e['Action'] == 'fail' for e in ordinary)
contracts = ['TestClientNegotiatesOnlyWithAdvertisedRendererOwner',
             'TestSnapshotAcceptanceAndRefusalPreserveOwnerTerminal',
             'TestSnapshotRendererNegotiationPreservesOwnedTerminal',
             'TestUnderlineRendererNegotiationPreservesOwnedTerminal',
             'TestSnapshotProfilePrecedesReplayAndPreservesJob',
             'TestSnapshotProfileTransitionClosesOnlyIncompatibleViewers',
             'TestSlowViewerCannotBlockTerminal']
assert all(any(e['Action'] == 'pass' and e.get('Test') == n for e in ordinary) for n in contracts)
skipped = [e['Test'] for e in ordinary if e['Action'] == 'skip' and e.get('Test', '').startswith('TestNativeProvider')]
assert all(n in skipped for n in ['TestNativeProviderScreenReattachmentAndContinuedInput',
                                  'TestNativeProviderContinuationState', 'TestNativeProviderSavedCursorCaptures'])
receipt['arm64Screen']['controlledProtocolParents'] = contracts
receipt['arm64Screen']['ordinaryNativeProviderSkips'] = skipped


def inventory(raw):
    ev = events(raw)
    assert not any(e['Action'] == 'fail' for e in ev)
    count = collections.Counter(e['Test'] for e in ev if e['Action'] == 'pass' and e.get('Test'))
    skip = collections.Counter(e['Test'] for e in ev if e['Action'] == 'skip' and e.get('Test'))
    assert set(count.values()) == {10}
    assert skip == {'TestDepDiffVT': 10, 'TestDepDiffResize': 10, 'TestGhosttyParkedCursorJoinsLines': 10}
    pkg = [e for e in ev if e['Action'] == 'pass' and not e.get('Test')]
    assert len(pkg) == 1 and pkg[0]['Elapsed'] < 600
    return count, skip, pkg[0]['Elapsed']


vt, receipt['vtRepetition'] = upload('windows-vt-repeat')
oldmeta = json.loads((ROOT / 'original-budget-correction-artifact.json').read_text())
assert oldmeta['workflow_run']['head_sha'] == OLD
assert oldmeta['digest'] == 'sha256:' + sha(OLD_ZIP.read_bytes())
with zipfile.ZipFile(OLD_ZIP) as oldzip:
    before, oldskip, oldelapsed = inventory(oldzip.read('webmux-vt-repeat.jsonl'))
after, newskip, newelapsed = inventory(vt.read('webmux-vt-repeat.jsonl'))
assert len(before) == 2186 and len(after) == 2193
assert all(after[n] == c for n, c in before.items()) and oldskip == newskip
assert after['TestGridMatchesUVBufferUnderRandomOperations'] == 10
context = decode(vt.read('webmux-vt-runtime-context.json'))
assert context['source'] == FINAL and context['goVersion'] == 'go version go1.27.4 windows/amd64'
receipt['vtRepetition'].update(originalCorrectedSource=OLD, originalCorrectedArtifact=oldmeta['id'],
    originalCorrectedDigest=oldmeta['digest'], originalIdentities=len(before), currentIdentities=len(after),
    originalPassEvents=sum(before.values()), currentPassEvents=sum(after.values()),
    eachIdentityPasses=10, originalElapsed=oldelapsed, currentElapsed=newelapsed,
    preservedSkips=dict(newskip), addedIdentities=sorted(after.keys()-before.keys()), worker=context)
receipt['originalFourMinuteFailures'] = []
failed_jobs = json.loads((ROOT / 'original-timeout-job-readback.json').read_text())
assert [j['id'] for j in failed_jobs] == [134586983884, 134589455925]
assert all(j['status'] == 'completed' and j['conclusion'] == 'failure' for j in failed_jobs)
for name, job in zip(['original-92a-vt.log', 'original-443-vt.log'], failed_jobs):
    data = (ROOT / name).read_bytes()
    assert b'panic: test timed out after 4m0s' in data and b'-count=10' in data
    assert job['head_sha'].encode() in data
    receipt['originalFourMinuteFailures'].append({'path': name, 'sha256': sha(data), 'bytes': len(data), 'job': job})

# The original correction changes one workflow only. Reverse it byte-for-byte.
assert git('diff-tree', '--no-commit-id', '--name-only', '-r', OLD).decode().splitlines() == ['.github/workflows/ci.yml']
old_workflow = git('show', OLD + ':.github/workflows/ci.yml').decode()
correction = '''        shell: pwsh
        working-directory: webmux/server/internal/thirdparty/vt
        run: |
          go test -json . -count=10 -timeout=10m | Tee-Object -FilePath "$env:RUNNER_TEMP/webmux-vt-repeat.jsonl"
          if ($LASTEXITCODE -ne 0) { throw 'Native Windows VT repetition failed' }

      - name: Preserve Windows terminal repetition receipts
        if: always()
        uses: actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9 # v7
        with:
          name: windows-vt-repeat
          path: ${{ runner.temp }}/webmux-vt-repeat.jsonl'''
assert old_workflow.count(correction) == 1
assert old_workflow.replace(correction, '        working-directory: webmux/server/internal/thirdparty/vt\n        run: go test . -count=10 -timeout=4m') == git('show', OLD + '^:.github/workflows/ci.yml').decode()
workflow = git('show', 'HEAD:.github/workflows/ci.yml').decode()
assert 'go test -json . -count=10 -timeout=10m | Tee-Object' in workflow
assert "if ($LASTEXITCODE -ne 0) { throw 'Native Windows VT repetition failed' }" in workflow
assert 'name: Preserve Windows terminal repetition receipts\n        if: always()' in workflow

# Native source and production dependencies remain byte-identical in this scope.
paths = ['webmux/server/internal/screenstate', 'webmux/server/internal/replayfilter',
         'webmux/server/internal/nativeprovider', 'webmux/server/internal/thirdparty/vt',
         'webmux/server/internal/thirdparty/ultraviolet', 'webmux/server/go.mod', 'webmux/server/go.sum',
         'webmux/package-lock.json', 'webmux/frontend/src/utils/terminalSnapshotProfile.ts',
         'webmux/frontend/src/utils/terminalHyperlinks.ts', 'webmux/frontend/src/components/Terminal.tsx',
         'tests/e2e/screen-replay.spec.ts', 'scripts/qualify-xterm-underline.mts']
receipt['identicalSourceObjects'] = []
for path in paths:
    a = git('rev-parse', FINAL + ':' + path).decode().strip()
    b = git('rev-parse', 'HEAD:' + path).decode().strip()
    assert a == b, path
    receipt['identicalSourceObjects'].append({'path': path, 'gitObject': a})

# Since the first corrected suite: only test diagnostics/reader cleanup/provenance.
base = 'webmux/server/internal/thirdparty/vt/'
changed = git('diff', '--name-only', OLD, 'HEAD', '--', base).decode().splitlines()
assert sorted(p.removeprefix(base) for p in changed) == ['WEBMUX.md', 'WEBMUX_UPSTREAM.json',
    'conform_report_test.go', 'grid_test.go', 'kitty_payload_test.go']
receipt['originalTestBodyPreservation'] = []
with tempfile.TemporaryDirectory(prefix='vt-acceptance-') as td:
    for filename, normalizations in [('conform_report_test.go', [('next, _ := replies(t, emu)', 'next := replies(emu)')]),
                                     ('kitty_payload_test.go', [('readResponses(t, e)', 'readResponses(e)')])]:
        inputs = []
        for rev, name in [(OLD, 'before'), ('HEAD', 'after')]:
            p = Path(td) / (name + '-' + filename)
            p.write_bytes(git('show', rev + ':' + base + filename)); inputs.append(str(p))
        bodies = json.loads(subprocess.check_output([GO, 'run', str(EXTRACTOR), '--', *inputs]))
        before = bodies[inputs[0]]; after = bodies[inputs[1]]
        for n, body in before.items():
            current = after[n]
            for new, old in normalizations:
                current = current.replace(new, old)
            assert body == current, n
        receipt['originalTestBodyPreservation'].append({'file': filename, 'originalTests': len(before),
            'normalizedOriginalSHA256': sha(json.dumps(before, sort_keys=True).encode()),
            'allOriginalBodiesPreserved': True})
grid = git('show', 'HEAD:' + base + 'grid_test.go').decode()
grid, n = re.subn(r'// diagnosticGridCell keeps.*?\n}\n\n', '', grid, count=1, flags=re.S)
assert n == 1 and grid.count('diagnosticGridCell(c)') == 6
assert grid.replace('diagnosticGridCell(c)', 'c') == git('show', OLD + ':' + base + 'grid_test.go').decode()
receipt['gridDiagnosticInversionExact'] = True
local = events((ROOT / 'component-race.jsonl').read_bytes())
assert not any(e['Action'] in ['fail', 'skip'] for e in local)
passes = [e for e in local if e['Action'] == 'pass' and e.get('Test')]
receipt['currentLocalRace'] = {'testAndSubtestPasses': len(passes),
    'topLevelPasses': sum('/' not in e['Test'] for e in passes), 'failSkip': 0,
    'logSHA256': sha((ROOT / 'component-race.jsonl').read_bytes())}
for prefix, expected in [('vt', 'Verified 241 upstream files; 19 explicitly patched files'),
                          ('uv', 'Verified 104 ultraviolet upstream files; 7 explicitly patched files')]:
    assert expected in (ROOT / (prefix + '-provenance.log')).read_text()
workflows = json.loads((ROOT / 'original-workflows-current-readback.json').read_text())
assert len(workflows) == 10 and all(w['head_sha'] == FINAL and w['conclusion'] == 'success' for w in workflows)
receipt['originalSuccessfulWorkflows'] = workflows
receipt['scope'] = 'Screen mirror, v2 renderer state and aggregate VT suite budget only. No current whole-main native run, native ARM browser, full coding lifecycle, AMX corruption repair, private Ice Lake deadline repair or release acceptance.'
(ROOT / 'receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
print('PASS: original ZIP/member hashes, AMD64/ARM64 capture+replay, protocol scope, ten-run inventory, source equality, original test assertions, local race/provenance and original ten workflow conclusions.')
