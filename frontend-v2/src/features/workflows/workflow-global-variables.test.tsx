import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { WorkflowGlobalVariables } from './workflow-global-variables'

describe('workflow global variables', () => {
  it('validates and saves typed values without mutating until saved', async () => {
    const save = vi.fn()
    render(<WorkflowGlobalVariables variables={[]} onSave={save} onClose={() => {}}/>)
    await userEvent.click(screen.getByRole('button', { name: '＋ 添加全局变量' }))
    expect(screen.getByRole('button', { name: '保存' })).toBeDisabled()
    await userEvent.type(screen.getByLabelText('变量名称 1'), 'global.count')
    await userEvent.selectOptions(screen.getByLabelText('变量类型 1'), 'number')
    await userEvent.type(screen.getByLabelText('变量值 1'), '42')
    expect(save).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: '保存' }))
    expect(save).toHaveBeenCalledWith([{ name: 'global.count', valueType: 'number', value: 42 }])
  })
  it('preserves untouched null and empty values and validates typed arrays and file references', async () => {
    const save = vi.fn()
    render(<WorkflowGlobalVariables variables={[{ name: 'global.empty', valueType: 'string', value: '' }, { name: 'global.null', valueType: 'any', value: null }, { name: 'global.files', valueType: 'array[file]', value: [{ kind: 'workspace_file', path: 'a.txt' }] }, { name: 'sys.workflow_id', valueType: 'string', extra: true }]} onSave={save} onClose={() => {}} />)
    await userEvent.click(screen.getByRole('button', { name: '保存' }))
    expect(save.mock.calls[0][0]).toEqual(expect.arrayContaining([{ name: 'global.empty', valueType: 'string', value: '' }, { name: 'global.null', valueType: 'any', value: null }, { name: 'global.files', valueType: 'array[file]', value: [{ kind: 'workspace_file', path: 'a.txt' }] }, { name: 'sys.workflow_id', valueType: 'string', extra: true }]))
    await userEvent.selectOptions(screen.getByLabelText('变量类型 3'), 'array[number]')
    fireEvent.change(screen.getByLabelText('变量值 3'), { target: { value: '[1, "bad"]' } })
    expect(screen.getByRole('button', { name: '保存' })).toBeDisabled()
  })

})
