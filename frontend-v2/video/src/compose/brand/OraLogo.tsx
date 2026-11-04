/**
 * The Ora mark (frontend-v2/public/favicon.svg) inlined, so its parts can move: `blink` closes the
 * eyes, 0 open and 1 shut.
 */
export function OraLogo({ size, blink = 0 }: { size: number; blink?: number }) {
  const lid = 1 - 0.92 * Math.min(Math.max(blink, 0), 1)
  return <svg width={size} height={size} viewBox="0 0 412 412" fill="none" style={{ display: 'block' }}>
    <rect width={412} height={412} rx={100} fill="#000" />
    <path d="M125 101C100.147 101 80 126.968 80 159L80 275C80 307.032 100.147 333 125 333L189.449 333L287 333C311.853 333 332 307.032 332 275L332 159C332 126.968 311.853 101 287 101L125 101Z" fill="#f2f2f2" fillRule="evenodd" />
    <ellipse rx={30.5} ry={36.5 * lid} cx={149.5} cy={195.5} fill="#000" />
    <ellipse rx={30} ry={36.5 * lid} cx={261} cy={195.5} fill="#000" />
    <path d={HAND} fill="#f2f2f2" fillRule="evenodd" transform="matrix(-0.930533,0.366208,0.367178,0.930151,341,177)" />
    <path d={HAND} fill="#f2f2f2" fillRule="evenodd" transform="matrix(0.930533,0.366208,-0.367178,0.930151,70,177)" />
    <path d="M307 109.5C307 126.897 299.389 141 290 141C280.611 141 273 126.897 273 109.5C273 102.305 280.345 105.144 282.537 99.84C285.643 92.3204 284.494 78 290 78C299.389 78 307 92.103 307 109.5Z" fill="#f2f2f2" fillRule="evenodd" />
    <path d="M34 31.5C34 48.897 26.3888 63 17 63C7.61116 63 0 48.897 0 31.5C0 24.3047 7.34546 27.1436 9.53667 21.84C12.6434 14.3204 11.4943 0 17 0C26.3888 0 34 14.103 34 31.5Z" fill="#f2f2f2" fillRule="evenodd" transform="matrix(-1,0,0,1,138,78)" />
  </svg>
}

const HAND = 'M34 52.1049C34 72.4815 26.3888 89 17 89C7.61116 89 0 72.4815 0 52.1049C0 31.7283 9.55158 0 18.9404 0C24.3852 0 27.2919 20.7655 30.4033 29.407C32.657 35.6666 34 43.5451 34 52.1049Z'
