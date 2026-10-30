import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

/** Raw HTML remains disabled. Images are omitted so documents cannot send background requests. */
export function MarkdownPreview({ content }: { content: string }) {
  return (
    <div className="settings-markdown">
      <Markdown
        remarkPlugins={[remarkGfm]}
        skipHtml
        components={{
          img: ({ alt }) => (
            <span className="text-muted-foreground">
              [图片：{alt || '未加载'}]
            </span>
          ),
          a: ({ href, children }) => (
            <a href={href} target="_blank" rel="noreferrer noopener">
              {children}
            </a>
          ),
        }}
      >
        {content || '暂无说明文档。'}
      </Markdown>
    </div>
  )
}
