import axios from 'axios'

export interface SSLDomain {
  id: number
  domain: string
  port: number
  memo: string
  created_at: string
}

export interface SSLCertStatus {
  domain: string
  port: number
  issuer: string
  not_after: string
  days_left: number
  status: string // ok, warning, critical, expired, error
  error?: string
  checked_at: string
}

export async function getSSLDomains(): Promise<SSLDomain[]> {
  const res = await axios.get('/api/ssl/domains')
  return res.data
}

export async function addSSLDomain(d: { domain: string; port: number; memo: string }): Promise<SSLDomain> {
  const res = await axios.post('/api/ssl/domains', d)
  return res.data
}

export async function deleteSSLDomain(id: number): Promise<void> {
  await axios.delete(`/api/ssl/domains/${id}`)
}

export async function getSSLStatus(): Promise<SSLCertStatus[]> {
  const res = await axios.get('/api/ssl/status')
  return res.data
}

export async function triggerSSLCheck(): Promise<SSLCertStatus[]> {
  const res = await axios.post('/api/ssl/check')
  return res.data
}
