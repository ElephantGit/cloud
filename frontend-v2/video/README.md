# 演示视频

用代码制作 Cloud v2 的演示视频：Playwright 按镜头脚本操作真实页面并录制，Remotion 在后期加上窗口框、自动运镜、合成光标、点击波纹、字幕和倍速提示，输出 4K（3840×2160、60 fps）成片和配音用的文字稿。独立的 npm 包，不进前端应用的依赖。

## 用法

先在仓库根目录启动镜头所属那一段的演示环境，并且每录一条都从干净状态开始，例如 B 段用 `task v2:demo:lead`。然后在本目录：

```sh
npm ci
npx playwright install chromium-headless-shell   # 第一次
npm run record -- lead-decompose                 # 录制，写入 out/lead-decompose/
npm run studio                                   # 在浏览器里预览和调整
npm run render -- lead-decompose out/lead-decompose.mp4
npm run check                                    # 类型检查和单元测试
```

- `VIDEO_BASE_URL`：录制时打开的前端地址，默认是该段单独演示命令的端口（A 5191、B 5192、C 5193）。连合成环境 `task v2:demo` 时设成 `http://127.0.0.1:5190`。
- `VIDEO_SCALE`：设备像素比，默认 2.5，页面按 1280×720 布局，录成 3200×1800。
- `PW_CHROMIUM_PATH`：指定录制用的 Chromium。Remotion 渲染时用 `--browser-executable` 指定。

`out/<镜头>/` 里有：

- `frames/`：录屏帧；
- `timeline.json`：光标、点击、镜头、倍速、字幕的时间线；
- `transcript.md` / `transcript.srt`：按成片时间排好的画面文字和旁白，交给配音。

## 结构

```text
src/
  timeline.ts          录制结果的格式，以及时间换算、镜头、光标的纯函数
  record/
    cli.ts             启动浏览器、登录、录屏，写出 out/<镜头>/
    director.ts        镜头脚本的动作：click、type、select、wait、focus、say、hold
    shot.ts            镜头的定义
    transcript.ts      从时间线生成文字稿
  shots/               镜头脚本，一个镜头一个文件，登记在 index.ts
  compose/             Remotion：窗口、运镜、光标、字幕
```

## 约定

- **镜头脚本只描述画面**：点哪里、输入什么、镜头看哪里、什么时候出哪句字幕和旁白。时间线由录制自动生成，不手填坐标和时间。
- **镜头跟着元素走**：`focus` 和各个动作记录的是元素在录制那一刻的位置，界面改版后重录即可，不用改脚本里的坐标。
- **真实执行用倍速**：`wait` 等待期间按 `rate` 加速，画面上显示倍速；文字稿里的时间是加速之后的成片时间。
- **录制必须用浏览器自己的设备像素比**：Chromium 的 screencast 对模拟视口一律按 CSS 尺寸截图，CSS `zoom` 又会把 `vh` 布局拉长，所以录制器关闭视口模拟，用 `--force-device-scale-factor` 和窗口大小启动 headless shell，启动后会校验实际的视口和像素比。
