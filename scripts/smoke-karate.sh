#!/usr/bin/env bash
# Explicitly provision a built CLI and the reviewed JAR; this smoke never
# installs or fetches a runner. It exercises a temporary project and loopback.
set -euo pipefail
: "${MULIX_BIN:?set the absolute path to the candidate mulix binary}"
: "${MULIX_KARATE_JAR:?set the absolute path to karate-agent-2.1.5.RC1.jar}"
python3 -I -B - <<'PY'
import hashlib
import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import subprocess
import sys
from tempfile import TemporaryDirectory
from threading import Thread

jar_sha = '9e5fc19c560fa6824aae07a52ec8ca11542ecf6e93bd9f823b039477525cf9e3'
binary = Path(os.environ['MULIX_BIN'])
jar = Path(os.environ['MULIX_KARATE_JAR'])
assert binary.is_absolute() and binary.is_file(), 'use an absolute candidate binary path'
assert jar.is_absolute() and jar.is_file(), 'use an absolute JAR path'
with jar.open('rb') as stream:
    assert hashlib.file_digest(stream, 'sha256').hexdigest() == jar_sha, 'wrong pinned JAR'

with TemporaryDirectory(prefix='mulix Karate smoke ') as directory:
    home = Path(directory)
    root = home / 'project 源码'
    temp = home / 'scratch'
    root.mkdir(); temp.mkdir()
    env = dict(os.environ, TMPDIR=str(temp), TEMP=str(temp), TMP=str(temp))
    env.pop('MULIX_SESSION_ID', None)

    def put(rel, text):
        target = root / rel
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text, encoding='utf-8')

    def cli(*args, ok=True):
        p = subprocess.run([str(binary), *args], cwd=root, env=env,
                           text=True, encoding='utf-8', errors='replace', capture_output=True, timeout=60)
        if (p.returncode == 0) != ok:
            traces = sorted(temp.rglob('karate-events.jsonl'), key=lambda f: f.stat().st_mtime)
            if traces:
                for line in traces[-1].read_text(encoding='utf-8').splitlines():
                    event = json.loads(line)
                    if event.get('type') == 'ERROR':
                        print('fixture error:', event.get('data'), file=sys.stderr)
            raise RuntimeError(f'{args}: exit={p.returncode}\n{p.stdout}\n{p.stderr}')
        return p.stdout

    def state_set(field, value):
        cli('state', 'set', field, value)

    def transition(event, ok=True):
        return cli('state', 'transition', event, ok=ok)

    class Service(BaseHTTPRequestHandler):
        def do_GET(self):
            data = json.loads((root / 'behavior.json').read_text(encoding='utf-8'))
            key = self.path[1:]
            self.send_response(200 if key in data else 404)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({'value': data.get(key)}).encode())

        def log_message(self, *ignored):
            pass

    with ThreadingHTTPServer(('127.0.0.1', 0), Service) as server:
        thread = Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            env['SMOKE_URL'] = f'http://127.0.0.1:{server.server_port}'
            cli('init', '--host', 'claude')
            cli('new', 'Karate scope smoke')
            ctx = json.loads(cli('context', 'show', '--json'))
            change = ctx['change']
            scratch = Path(ctx['scratch_dir']) / 'verification'
            runtime = Path(ctx['runtime_dir']).relative_to(root).as_posix()
            def karate_runs():
                return set(scratch.glob('*-karate'))

            def cases(run):
                raw = run / 'output/karate-json/karate-events.jsonl'
                assert (run / 'karate-agent.log').is_file(), 'no separate rolling log'
                return [(e['data']['feature'], e['data']['name'], e['data']['passed'])
                        for e in map(json.loads, raw.read_text(encoding='utf-8').splitlines())
                        if e['type'] == 'SCENARIO_EXIT' and e['data']['callDepth'] == 0]

            def scenario_run(*args, ok=True):
                before = karate_runs()
                cli(*args, ok=ok)
                new = karate_runs() - before
                assert len(new) == 1, f'expected one Karate run, observed {len(new)}'
                return cases(new.pop())

            spec = f'docs/specs/{change}/spec.md'
            change_dir = f'docs/changes/{change}'
            plan = f'{change_dir}/tasks.md'
            put(spec, '# Smoke specification\nFR-001: Preserve API behavior.\n\n```mulix-acceptance\ncriteria:\n  - id: AC-001\n    requirement: FR-001\n    expected: Each endpoint returns its specified value.\n```\n')
            state_set('spec_path', spec); transition('spec-complete')
            state_set('clarify_skipped', 'true'); transition('clarify-skipped')
            state_set('design_track', 'bounded'); state_set('design_approved', 'true'); transition('design-approved')
            first = 'features/task-1/first case.feature'
            second = 'features/task-2/second.feature'
            final = 'features/final/integration.feature'
            def case(id, file, scenario, task=None):
                row = dict(id=id, criterion='AC-001', suite='karate', file=f'{runtime}/tests/{file}',
                           test=f'{runtime}/tests/{file}#{scenario}')
                if task:
                    row.update(task=task, point='specified result', given='the local service',
                               when='getting its endpoint', then='its specified value is returned')
                return row
            docs_case = dict(id='TC-003', criterion='AC-001', task=3, point='fixture docs', given='the API notes',
                             when='checking the documented route', then='its spelling is correct',
                             suite='docs', test='documented route', file='check-docs.py')
            matrix = dict(tasks=[dict(number=1, kind='code'), dict(number=2, kind='code'),
                                 dict(number=3, kind='documentation', reason='check the actual API notes')],
                          cases=[case('TC-001', first, '值保持不变', 1), case('TC-002', second, 'second', 2), docs_case],
                          verify_cases=[case('TC-004', final, 'joined flow')])
            put(plan, '# Tasks\n\n```mulix-tests\n'+json.dumps(matrix, ensure_ascii=False)+'\n```\n\n## Task 1: Implement first endpoint\n\n## Task 2: Implement second endpoint\n\n## Task 3: Check docs\n')
            put('.mulix/quality.yaml', json.dumps(dict(schema='mulix.quality.v1', suites=[
                dict(id='karate', format='karate-jsonl', karate=dict(version='2.1.5.RC1', sha256=jar_sha, environment='smoke', env=['SMOKE_URL'])),
                dict(id='docs', format='json', cwd='.', command=[sys.executable, '-I', '-B', 'check-docs.py']),
            ])))
            state_set('tasks_path', plan); transition('tasks-complete'); state_set('execution_method', 'inline')
            assert (root / runtime / 'tests').is_dir()
            for rel, name, endpoint, expected in [(first,'值保持不变','first',2),(second,'second','second',3),(final,'joined flow','joined',True)]:
                put(runtime+'/tests/'+rel, f'''Feature: {endpoint}
Scenario: {name}
* def prepared = call read('this:../../helpers/identity.feature')
Given url baseUrl
And path '{endpoint}'
When method get
Then status 200
And match prepared.ok == true
And match response.value == {json.dumps(expected)}
''')
            put(runtime+'/tests/helpers/identity.feature', 'Feature: reusable check\nScenario: nested calculation\n* def ok = 1 + 1 == 2\n')
            put(runtime+'/tests/helpers/karate-config.js', "function fn() { return { baseUrl: karate.sysenv('SMOKE_URL') }; }\n")
            put('notes.md', 'Endpoint: /first\n')
            put('check-docs.py', "import json, sys\nfrom pathlib import Path\nok = Path('notes.md').read_text().strip() == 'Endpoint: /first'\nprint(json.dumps({'name': 'documented route', 'status': 'pass' if ok else 'fail'}))\nsys.exit(not ok)\n")
            original = {f.relative_to(root).as_posix(): f.read_bytes() for f in (root/runtime/'tests').rglob('*') if f.is_file()}
            data = dict(first=0, second=0, joined=False)
            put('behavior.json', json.dumps(data))
            for n, key, value, file, name in [('1','first',2,first,'值保持不变'),('2','second',3,second,'second')]:
                assert scenario_run('test','run','--task',n,'--stage','red') == [(file,name,False)]
                data[key] = value; put('behavior.json', json.dumps(data))
                assert scenario_run('test','run','--task',n,'--stage','green') == [(file,name,True)]
                before = karate_runs()
                assert 'current green receipt' in cli('test','receipt','--task',n)
                assert karate_runs() == before, 'closure relaunched JAR'
            before = karate_runs()
            cli('test','run','--task','3','--stage','check')
            cli('test','receipt','--task','3')
            assert karate_runs() == before, 'non-Karate task ran the JAR'
            put(f'{runtime}/sdd/tasks/progress.md', f'# SDD progress — plan: {plan}\n- Task 1: complete\n- Task 2: complete\n- Task 3: complete\n')
            transition('build-complete')
            assert karate_runs() == before
            put(f'{change_dir}/report.md', '# Synthetic smoke review\n```mulix-review\nreviewer: smoke fixture\nresult: approved\nblocking: []\n```\n')
            failed = scenario_run('verify', ok=False)
            assert sorted(failed) == sorted([(first,'值保持不变',True),(second,'second',True),(final,'joined flow',False)])
            transition('verify-pass', ok=False)
            transition('verify-fail')
            data['joined'] = True; put('behavior.json', json.dumps(data))
            transition('build-complete')
            assert sorted(scenario_run('verify')) == sorted([(first,'值保持不变',True),(second,'second',True),(final,'joined flow',True)])
            before = karate_runs()
            transition('verify-pass'); state_set('archive_confirmation', 'confirmed')
            data['unrelated'] = True; put('behavior.json', json.dumps(data))
            transition('archived', ok=False)
            assert karate_runs() == before, 'stale archive unexpectedly launched a runner'
            assert len(scenario_run('verify')) == 3
            before = karate_runs(); transition('archived')
            assert karate_runs() == before, 'archive relaunched JAR'
            actual = {f.relative_to(root).as_posix(): f.read_bytes() for f in (root/runtime/'tests').rglob('*') if f.is_file()}
            assert actual == original, 'runtime tests received generated files or changed bytes'
            print('Karate RC1: 2 scoped RED/GREEN pairs, 1 non-Karate CHECK, full 3-scenario verify, regression/stale archive rejected, receipts closed without rerunning.')
        finally:
            server.shutdown()
            thread.join(timeout=5)
PY
