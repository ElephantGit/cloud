#!/usr/bin/env node
// Demo-only quality scan for the recording fixture. It reads the repository, applies a few fixed
// rules and prints one JSON document. Nothing is installed, fetched or written back.
//
//   node scripts/scan.mjs --mode incremental   # files listed in release.json
//   node scripts/scan.mjs --mode full          # every source file and its test coverage
import { readFileSync, readdirSync, existsSync } from 'node:fs'
import path from 'node:path'

const mode = process.argv.includes('--mode') ? process.argv[process.argv.indexOf('--mode') + 1] : 'incremental'
if (mode !== 'incremental' && mode !== 'full') {
  console.error('usage: node scripts/scan.mjs --mode incremental|full')
  process.exit(2)
}

const root = path.resolve(import.meta.dirname, '..')
const release = JSON.parse(readFileSync(path.join(root, 'release.json'), 'utf8'))
const sourceFiles = readdirSync(path.join(root, 'src'))
  .filter((name) => name.endsWith('.mjs'))
  .map((name) => path.posix.join('src', name))
  .sort()

const changed = sourceFiles.filter((file) => release.changed.includes(file))
const scanned = mode === 'incremental' ? changed : sourceFiles
const findings = []
let rules = 0

for (const file of scanned) {
  const text = readFileSync(path.join(root, file), 'utf8')
  // Rule: credentials committed next to the code they authenticate.
  rules++
  const token = text.match(/(?:API_TOKEN|ACCESS_TOKEN|TOKEN)\s*=\s*['"][^'"]+['"]/)
  if (token) {
    findings.push({
      key: 'src:hardcoded-token',
      title: '源码中存在硬编码令牌',
      description: '令牌应与代码分离，改为从部署环境读取',
      evidence: `${file}:${lineOf(text, token.index)} 常量里直接写入了令牌`,
      severity: '高',
      module: file,
    })
  }
  // Rule: dynamic evaluation of template input.
  rules++
  const evalCall = text.match(/\beval\(/)
  if (evalCall) {
    findings.push({
      key: 'src:eval-usage',
      title: '使用 eval 执行动态代码',
      description: '改为模板函数或显式分支，避免执行拼接出来的代码',
      evidence: `${file}:${lineOf(text, evalCall.index)} 用 eval 渲染模板`,
      severity: '中',
      module: file,
    })
  }
}

if (mode === 'full') {
  // Rule: every source module has a matching test module.
  rules++
  for (const file of sourceFiles) {
    const test = path.posix.join('test', path.basename(file).replace(/\.mjs$/, '.test.mjs'))
    if (!existsSync(path.join(root, test))) {
      findings.push({
        key: 'test:uncovered-module',
        title: '模块缺少测试覆盖',
        description: '新增模块需要补上对应的 node --test 用例',
        evidence: `${file} 没有 ${test}`,
        severity: '中',
        module: 'test',
      })
    }
  }
}

findings.sort((a, b) => a.key.localeCompare(b.key))
console.log(
  JSON.stringify(
    {
      mode,
      baseline: release.baseline,
      scanned: { files: scanned.length, rules, changed: changed.length },
      findings,
    },
    null,
    2,
  ),
)

function lineOf(text, index) {
  return text.slice(0, index).split('\n').length
}
