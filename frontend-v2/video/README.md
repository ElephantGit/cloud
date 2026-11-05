# 演示视频

用代码制作 Cloud v2 的演示视频：Playwright 按镜头脚本操作真实页面并录制，Remotion 在后期加上窗口框、自动运镜、合成光标、点击波纹、字幕和倍速提示，输出 4K（3840×2160、60 fps）成片和配音用的文字稿。独立的 npm 包，不进前端应用的依赖。

## 用法

先在仓库根目录启动镜头所属那一段的演示环境，并且每录一条都从干净状态开始，例如 `task v2:demo:lead`。只有章节页的镜头不需要环境。然后在本目录：

```sh
npm ci
npx playwright install chromium-headless-shell   # 第一次
npm run record -- lead-decompose                 # 录制，写入 out/lead-decompose/
npm run studio                                   # 在浏览器里预览和调整
npm run render -- lead-decompose out/lead-decompose.mp4
npm run render -- intro out/intro.mp4            # 片头，片尾是 outro，不需要录制
npm run film                                     # 把 out/ 里渲染好的片头、各镜头、片尾拼成 out/film.mp4
npm run check                                    # 类型检查和单元测试
```

- `VIDEO_BASE_URL`：录制时打开的前端地址，默认是该段单独演示命令的端口（A 5191、B 5192、C 5193）。连合成环境 `task v2:demo` 时设成 `http://127.0.0.1:5190`。
- `VIDEO_SCALE`：设备像素比，默认 2.5，页面按 1280×720 布局，录成 3200×1800。
- `PW_CHROMIUM_PATH`：指定录制用的 Chromium。Remotion 渲染时用 `--browser-executable` 指定。

`out/<镜头>/` 里有：

- `frames/`：录屏帧；
- `timeline.json`：光标、点击、镜头、倍速、定格、讲解和字幕的时间线；
- `transcript.md` / `transcript.srt`：按成片时间排好的文字稿，标出画面不够、定格补时的句子。

`npm run film` 还会写出整片的 `out/film.md` 和 `out/film.srt`，时间码已经算上片头和前面的镜头。默认按 `src/shots/index.ts` 的登记顺序拼接全部镜头，`npm run film -- a b` 只拼指定的镜头。每个镜头需要 `out/<镜头>.mp4` 和 `out/<镜头>/timeline.json`。

渲染结果只能放在 `out/`，它在 `.gitignore` 里，视频不进仓库。

## 写镜头脚本

先写文字稿，确认后再写脚本。例子见 `src/shots/lead-decompose.ts`。

| 动作 | 用途 |
|---|---|
| `d.say({ narration })` | 旁白和字幕。这句读完之前下一句不会开始，画面不够时自动定格 |
| `d.slide({ eyebrow, title, subtitle, narration, items })` | 全屏章节页：每段开头和少数纯概念，每段 1–2 次 |
| `await d.explain({ title, narration, items, focus, zoom, side })` | 窗口让位：主力讲解方式，窗口定格到一侧，卡片随旁白逐条出现；`focus` 在小窗口里推近，保证文字看得清 |
| `await d.mark(target, { label, narration })` | 高亮框和短标签，不放段落文字 |
| `await d.focus(target, zoom)` | 唯一改变缩放的动作。`d.focus('page', 1.3)` 是页面操作时的默认机位 |
| `d.click` / `d.type` / `d.select` | 镜头只平移，目标已在画面内就不动 |
| `await d.wait(until, { rate })` | 等真实执行，按倍速播放 |
| `await d.switchAccount(account, path, slide, ready)` | 在章节页后面切换账号，切换耗时从成片里剪掉 |

`d.slide` 和 `d.explain` 只占成片时间，录制时不等待，所以可以在真实执行开始后马上讲解。

录制结束时，终端会提示定格补时的句子和离上次变焦不到 3 秒的变焦，按提示调整脚本。

## 结构

```text
src/
  timeline.ts          录制结果的格式，以及时间换算、镜头、光标、旁白时长的纯函数
  record/
    cli.ts             启动浏览器、登录、录屏，写出 out/<镜头>/
    director.ts        镜头脚本的动作
    shot.ts            镜头的定义
    transcript.ts      从时间线生成文字稿
  shots/               镜头脚本，一个镜头一个文件，登记在 index.ts，登记顺序就是成片顺序
  film/                把渲染好的片段拼成整片，合并文字稿
  compose/             Remotion：窗口、运镜、光标、字幕
    Panels.tsx         章节页、让位讲解的卡片、高亮框
    Intro.tsx          片头：点阵和连线背景、logo 和 Ora Cloud 字样（合成 intro）
    Outro.tsx          片尾：logo、字样和标语（合成 outro）
    brand/             片头片尾共用的 logo、背景、颗粒和 logo 字样组合
```

## 约定

- **镜头脚本只描述画面**：点哪里、输入什么、镜头看哪里、什么时候出哪句字幕和旁白。时间线由录制自动生成，不手填坐标和时间。
- **镜头跟着元素走**：`focus` 和各个动作记录的是元素在录制那一刻的位置，界面改版后重录即可，不用改脚本里的坐标。
- **快速操作不起落镜头**：动作只平移，缩放只由 `focus` 改变，看着不晕。
- **旁白决定节奏**：按每秒约 4.5 字算每句的时长，倍速段按成片时长计。字幕版直接显示旁白全文。
- **旁白不写模型输出的具体数字**：真实模型每次的结果可能不同。
- **录制字体统一**：录制器把页面字体换成本包的 Geist 和 Noto Sans SC，不同机器录出来的文字一样。
- **真实执行用倍速**：`wait` 等待期间按 `rate` 加速，画面上显示倍速；文字稿里的时间是加速之后的成片时间。
- **录制必须用浏览器自己的设备像素比**：Chromium 的 screencast 对模拟视口一律按 CSS 尺寸截图，CSS `zoom` 又会把 `vh` 布局拉长，所以录制器关闭视口模拟，用 `--force-device-scale-factor` 和窗口大小启动 headless shell，启动后会校验实际的视口和像素比。
