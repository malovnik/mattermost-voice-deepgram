import {execFileSync} from 'node:child_process';
import {readFileSync} from 'node:fs';
import assert from 'node:assert/strict';
const m = JSON.parse(readFileSync('plugin.json', 'utf8'));
const files = execFileSync('tar', ['-tzf', process.argv[2]], {encoding:'utf8'}).split('\n');
for (const path of ['plugin.json', m.webapp.bundle_path, m.icon_path, ...Object.values(m.server.executables)]) {
  assert(files.includes(`${m.id}/${path}`), `Missing ${path}`);
}
assert(!files.some((p) => /node_modules|\.env|\.DS_Store|__MACOSX|\/\._/.test(p)), 'Unexpected files');
const packed = JSON.parse(execFileSync('tar', ['-xOzf',process.argv[2],`${m.id}/plugin.json`],{encoding:'utf8'}));
assert.deepEqual(packed,m);
console.log(`Validated installable bundle: ${m.id} ${m.version}`);
