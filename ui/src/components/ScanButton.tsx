// The "Scan now" button — triggers a network scan when clicked.
// While a scan is running it shows a spinner and is disabled.

interface Props {
  scanning: boolean  // true while a scan is in progress
  onScan: () => void // called when the user clicks the button
}

export function ScanButton({ scanning, onScan }: Props) {
  return (
    <button
      onClick={onScan}
      disabled={scanning} // can't click again while already scanning
      className={[
        'flex items-center gap-2 px-5 py-2.5 rounded-lg font-semibold text-sm transition-all',
        scanning
          ? 'bg-slate-700 text-slate-400 cursor-not-allowed' // greyed out while scanning
          : 'bg-cyan-600 hover:bg-cyan-500 active:bg-cyan-700 text-white shadow-lg shadow-cyan-900/30',
      ].join(' ')}
    >
      {scanning ? (
        <>
          {/* Spinning circle shown while scan is running */}
          <span className="w-4 h-4 border-2 border-slate-500 border-t-slate-300 rounded-full animate-spin" />
          Scanning…
        </>
      ) : (
        <>
          {/* Static circle shown when ready */}
          <span className="w-4 h-4 rounded-full border-2 border-white/60" />
          Scan now
        </>
      )}
    </button>
  )
}
