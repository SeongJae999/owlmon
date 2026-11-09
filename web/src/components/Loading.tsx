export default function Loading({ label, rows = 4 }: { label: string; rows?: number }) {
  return (
    <div role="status" aria-live="polite" className="p-6 space-y-3">
      <p className="text-xs font-medium text-base-500">{label}</p>
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="h-10 rounded-lg bg-base-800/70 motion-safe:animate-pulse" />
      ))}
    </div>
  )
}
