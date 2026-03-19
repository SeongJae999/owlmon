import axios from 'axios'

export interface SNMPDevice {
  ID: number
  Name: string
  IP: string
  Community: string
  Port: number
}

export interface InterfaceStats {
  Index: number
  Name: string
  OperUp: boolean
  InBytes: number
  OutBytes: number
  InBps: number
  OutBps: number
}

export interface DeviceStatus {
  Device: SNMPDevice
  Up: boolean
  UptimeSec: number
  Interfaces: InterfaceStats[]
  CollectedAt: string
}

export async function getSNMPDevices(): Promise<SNMPDevice[]> {
  const res = await axios.get('/api/snmp/devices')
  return res.data ?? []
}

export async function addSNMPDevice(device: Omit<SNMPDevice, 'ID'>): Promise<SNMPDevice> {
  const res = await axios.post('/api/snmp/devices', device)
  return res.data
}

export async function deleteSNMPDevice(id: number): Promise<void> {
  await axios.delete(`/api/snmp/devices/${id}`)
}

export async function getSNMPStatus(): Promise<DeviceStatus[]> {
  const res = await axios.get('/api/snmp/status')
  return res.data ?? []
}
