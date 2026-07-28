import { ESLint } from 'eslint'

// Existing frontend debt is tracked as a per-rule ceiling. CI fails when a
// rule exceeds its recorded count or a new rule appears, while allowing the
// architecture work to enable lint immediately. Lower a ceiling whenever
// violations are fixed; `pnpm lint:strict` shows the complete backlog.
const baseline = new Map([
  ['error:react-hooks/purity', 2],
  ['error:react-hooks/set-state-in-effect', 31],
  ['error:@typescript-eslint/no-explicit-any', 29],
  ['error:no-empty', 5],
  ['error:@typescript-eslint/no-unused-vars', 4],
  ['error:no-case-declarations', 4],
  ['error:react-hooks/immutability', 2],
  ['warning:react-hooks/exhaustive-deps', 12],
  ['warning:react-hooks/incompatible-library', 2],
])

const eslint = new ESLint()
const results = await eslint.lintFiles(['.'])
const current = new Map()

for (const result of results) {
  for (const message of result.messages) {
    const severity = message.severity === 2 ? 'error' : 'warning'
    const key = `${severity}:${message.ruleId || 'fatal'}`
    current.set(key, (current.get(key) || 0) + 1)
  }
}

const regressions = []
for (const [key, count] of current) {
  const allowed = baseline.get(key) || 0
  if (count > allowed) regressions.push(`${key}: ${count} > ${allowed}`)
}

const errors = results.reduce((total, result) => total + result.errorCount, 0)
const warnings = results.reduce((total, result) => total + result.warningCount, 0)
if (regressions.length > 0) {
  const formatter = await eslint.loadFormatter('stylish')
  console.error(formatter.format(results))
  console.error(`Lint baseline regressions:\n${regressions.join('\n')}`)
  process.exitCode = 1
} else {
  console.log(`ESLint baseline passed (${errors} existing errors, ${warnings} existing warnings; no regression)`)
}
