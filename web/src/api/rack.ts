import axios from 'axios'

export interface Rack {
  name: string
  room: string
  height: number
}

export async function fetchRacks(): Promise<Rack[]> {
  const res = await axios.get<Rack[]>('/api/racks')
  return res.data ?? []
}

export async function saveRack(rack: Rack): Promise<void> {
  await axios.put('/api/racks', rack)
}

export async function deleteRack(name: string): Promise<void> {
  await axios.delete('/api/racks', { params: { name } })
}
