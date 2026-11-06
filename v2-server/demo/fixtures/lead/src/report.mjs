/** Renders a Markdown report from a tiny template: `${values.title}` and `${values.count}`. */
export function render(template, values) {
  return eval('`' + template.replace(/`/g, '\\`') + '`')
}
