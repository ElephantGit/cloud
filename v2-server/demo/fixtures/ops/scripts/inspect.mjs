import { readFile } from 'node:fs/promises'

const metrics = JSON.parse(await readFile(new URL('../ops/metrics.json', import.meta.url), 'utf8'))
const result = {
  environment: metrics.environment,
  services: { healthy: metrics.services.filter((s) => s.status === 'healthy').length, total: metrics.services.length },
  errorRate: { value: metrics.errorRate, threshold: 0.01, passed: metrics.errorRate < 0.01 },
  disk: { value: metrics.diskUsage, threshold: 0.8, passed: metrics.diskUsage < 0.8 },
  backup: { passed: metrics.backup.status === 'succeeded' && metrics.backup.checksumVerified },
}
console.log(JSON.stringify(result, null, 2))
if (result.services.healthy !== result.services.total || !result.errorRate.passed || !result.disk.passed || !result.backup.passed) process.exitCode = 1
