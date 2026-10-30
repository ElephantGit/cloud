# Settings

设置模块包含成员、模型连接、主题、Plugins 和 Skills 五个独立子路由，默认进入成员页。页面沿用 V2 的组件和语义颜色，不引入其他 UI 库。

## 数据边界

除真实登录状态与浏览器主题偏好之外，四个管理页面全部是明确标注的演示。不会调用成员、模型、插件或技能 HTTP 接口，不会安装软件、执行模型、邀请用户或修改聊天选择。不存在请求失败后自动切换假数据的逻辑。

`data/repository.ts` 是本机存储边界，键包含版本、当前用户 ID 和演示空间 ID。初次读取创建示例；修改先校验 revision 并成功写入 localStorage，再更新 TanStack Query 缓存。保存失败和损坏数据会报错，不会自动清空。用户明确确认后才能重置。此数据不跨设备同步，不是正式服务端存储或授权实现。

- 插件复用生成的 `PluginCatalogEntry` 与 `SpacePlugin`；以 `sourceNamespace/identifier` 对应，目录行 ID 与安装行 ID 不混用。安装意图与结果分开保存；模拟安装直接完成本地状态变化，不伪造下载进度。只演示非 Agent 插件；合集只读。
- 技能是独立的说明文档，保留 namespace、source、availability、content。自建技能可修改；插件技能只读，安装/移除改变可用性，移除后仍能预览文档。预览不执行 HTML 或加载远程图片。
- 成员复用生成类型，演示管理员权限、最后管理员保护、停用保留历史及申请审批。链接是当前浏览器的预览地址，没有真实邀请令牌。企业目录不在本模块内。
- 模型连接复用生成类型，维护元数据、多模型和默认选择；只模拟 credentialConfigured，不提供真实 API Key 输入。默认连接失效时提示重新选择，不悄悄替换模型。

## 结构

- `page.tsx`、`navigation.ts`、`index.ts`：设置容器、导航、懒加载子路由。
- `members/`、`model-connections/`、`theme/`、`plugins/`、`skills/`：各页界面、表单与测试。
- `components/`：设置页面共用的状态、确认、详情和 Markdown 展示。
- `data/`：独立类型、示例、持久化边界与领域操作；只通过这里更新演示状态。

主题由应用级 next-themes 管理，默认浅色，浏览器键为 `ora-v2-theme`。只有用户明确选择“跟随系统”后才读取系统外观；已有的浅色、深色或跟随系统偏好继续生效。Vite 的 createRoot 模式不会执行组件里的 SSR 脚本，因此 `main.tsx` 在挂载前应用初始主题，provider 的 SSR 脚本显式保持 inert；之后由 provider 管理主题及系统变化。现有定时页仅补深色变量适配。

## 验证

`npm run check` 检查类型、Oxlint、单元测试、客户端一致性与构建。`e2e/settings.spec.ts` 使用真实开发者登录后端，覆盖五页路由、浏览器持久化、错误恢复、权限相关交互、主题、375/768/1440 布局与截图。资源操作验收的是本机演示，不代表后端能力已经实现。
