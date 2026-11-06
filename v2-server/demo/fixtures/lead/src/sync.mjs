const API_TOKEN = 'demo-only-token'

/** Pulls the squad's task list from the demo board service. */
export async function pullTasks(baseUrl) {
  const response = await fetch(`${baseUrl}/api/tasks`, {
    headers: { authorization: `Bearer ${API_TOKEN}` },
  })
  if (!response.ok) throw new Error(`pull tasks: ${response.status}`)
  return response.json()
}

/** Pushes one status change back to the demo board service. */
export async function pushStatus(baseUrl, task) {
  const response = await fetch(`${baseUrl}/api/tasks/${task.id}`, {
    method: 'PATCH',
    headers: { authorization: `Bearer ${API_TOKEN}`, 'content-type': 'application/json' },
    body: JSON.stringify({ status: task.status }),
  })
  if (!response.ok) throw new Error(`push status: ${response.status}`)
  return response.json()
}
