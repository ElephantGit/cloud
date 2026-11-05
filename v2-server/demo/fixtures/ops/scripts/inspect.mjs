import { readFile } from 'node:fs/promises'

const metrics = JSON.parse(await readFile(new URL(process.argv.includes('--healthy') ? '../ops/metrics-healthy.json' : '../ops/metrics.json', import.meta.url), 'utf8'))
const result = {
  environment: metrics.environment,
  services: { healthy: metrics.services.filter((s) => s.status === 'healthy').length, total: metrics.services.length },
  errorRate: { value: metrics.errorRate, threshold: 0.01, passed: metrics.errorRate < 0.01 },
  disk: { value: metrics.diskUsage, threshold: 0.8, passed: metrics.diskUsage < 0.8 },
  backup: { passed: metrics.backup.status === 'succeeded' && metrics.backup.checksumVerified },
}

result.findings = []
if (result.services.healthy !== result.services.total) result.findings.push({key:'services:availability',title:'服务健康检查异常',description:'核对不可用服务',evidence:`健康服务 ${result.services.healthy}/${result.services.total}`,severity:'高',module:'服务'})
if (!result.backup.passed) result.findings.push({key:'backup:checksum',title:'备份或校验失败',description:'检查最近备份',evidence:JSON.stringify(metrics.backup),severity:'高',module:'备份'})
if (!result.errorRate.passed) result.findings.push({key:'api:error-rate',title:'接口错误率超过阈值',description:'错误率需要排查',evidence:`错误率 ${metrics.errorRate}，阈值 0.01`,severity:'高',module:'API'})
if (!result.disk.passed) result.findings.push({key:'storage:root-disk',title:'磁盘占用超过阈值',description:'检查磁盘增长与清理策略',evidence:`占用 ${metrics.diskUsage}，阈值 0.8`,severity:'中',module:'存储'})
if (metrics.maintenanceNotice) result.findings.push({key:'service:maintenance-notice',title:'维护窗口内出现重启提示',description:'请结合维护记录确认是否需要处理',evidence:'计划维护已完成，服务当前健康；该提示可能无需提单',severity:'低',module:'服务'})
console.log(JSON.stringify(result, null, 2))
