// 엑셀에서 "CSV"로 저장하면 한글이 CP949(EUC-KR)로 나오는 경우가 많다 — UTF-8로 안 읽히면 EUC-KR로 다시 읽음
export function decodeText(buf: ArrayBuffer): string {
  // 엑셀 "유니코드 텍스트(*.txt)"는 UTF-16 — BOM으로 구분
  const b = new Uint8Array(buf.slice(0, 2))
  if (b[0] === 0xff && b[1] === 0xfe) return new TextDecoder('utf-16le').decode(buf)
  if (b[0] === 0xfe && b[1] === 0xff) return new TextDecoder('utf-16be').decode(buf)
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(buf)
  } catch {
    return new TextDecoder('euc-kr').decode(buf)
  }
}

// 엑셀에서 복사해 붙여넣으면 탭 구분, 파일은 쉼표 구분 — 첫 줄에 탭이 있으면 탭으로 본다
export function parseTable(text: string): string[][] {
  const src = text.replace(/^\uFEFF/, '')
  const firstLine = src.split(/\r?\n/).find(l => l.trim() !== '') ?? ''
  const sep = firstLine.includes('\t') ? '\t' : ','
  const rows: string[][] = []
  let row: string[] = []
  let cell = ''
  let quoted = false
  for (let i = 0; i < src.length; i++) {
    const c = src[i]
    if (quoted) {
      if (c === '"' && src[i + 1] === '"') { cell += '"'; i++ }
      else if (c === '"') quoted = false
      else cell += c
    } else if (c === '"' && cell === '') quoted = true
    else if (c === sep) { row.push(cell); cell = '' }
    else if (c === '\n' || c === '\r') {
      if (c === '\r' && src[i + 1] === '\n') i++
      row.push(cell); rows.push(row); row = []; cell = ''
    } else cell += c
  }
  if (cell !== '' || row.length) { row.push(cell); rows.push(row) }
  // 빈 줄도 남겨 둠 — 걸러내면 미리보기 줄 번호가 엑셀 행 번호와 어긋남
  return rows.map(r => r.map(c => c.trim()))
}

export function toCSV(rows: string[][]): string {
  const esc = (c: string) => /[",\r\n]/.test(c) ? `"${c.replace(/"/g, '""')}"` : c
  // BOM을 붙여야 엑셀이 UTF-8로 열어 한글이 깨지지 않음
  return '\uFEFF' + rows.map(r => r.map(esc).join(',')).join('\r\n') + '\r\n'
}
