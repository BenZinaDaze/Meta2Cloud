export default function BrandMark({ className = '', compact = false }: { className?: string; compact?: boolean }) {
  return (
    <svg
      viewBox="0 0 64 64"
      aria-hidden="true"
      className={className}
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      <defs>
        <linearGradient id="brand-bg" x1="12" y1="8" x2="54" y2="58" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor="#17314f" />
          <stop offset="1" stopColor="#091523" />
        </linearGradient>
        <linearGradient id="brand-folder" x1="18" y1="23" x2="45" y2="45" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor="#ddb06f" />
          <stop offset="1" stopColor="#b67a36" />
        </linearGradient>
        <linearGradient id="brand-meta" x1="14" y1="20" x2="50" y2="46" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor="#7bc4ff" />
          <stop offset="1" stopColor="#4a82d3" />
        </linearGradient>
        <radialGradient id="brand-gold-glow" cx="0" cy="0" r="1" gradientUnits="userSpaceOnUse" gradientTransform="translate(44 18) rotate(135) scale(26 25)">
          <stop stopColor="#c8924d" stopOpacity=".34" />
          <stop offset="1" stopColor="#c8924d" stopOpacity="0" />
        </radialGradient>
        <radialGradient id="brand-blue-glow" cx="0" cy="0" r="1" gradientUnits="userSpaceOnUse" gradientTransform="translate(18 16) rotate(60) scale(24 22)">
          <stop stopColor="#5f98e8" stopOpacity=".35" />
          <stop offset="1" stopColor="#5f98e8" stopOpacity="0" />
        </radialGradient>
      </defs>

      <rect x="5.5" y="5.5" width="53" height="53" rx="17" fill="url(#brand-bg)" />
      <rect x="5.5" y="5.5" width="53" height="53" rx="17" stroke="rgba(167,198,235,0.18)" />
      <rect x="7.5" y="7.5" width="49" height="49" rx="15" stroke="rgba(200,146,77,0.18)" />
      <rect x="5.5" y="5.5" width="53" height="53" rx="17" fill="url(#brand-blue-glow)" />
      <rect x="5.5" y="5.5" width="53" height="53" rx="17" fill="url(#brand-gold-glow)" />

      <path
        d="M18.5 24.5h10.4l2.8 2.9h14.8c1.2 0 2.1 1 2.1 2.1v11.8c0 1.2-.9 2.2-2.1 2.2h-29c-1.2 0-2.1-1-2.1-2.2V26.6c0-1.2.9-2.1 2.1-2.1Z"
        fill="url(#brand-folder)"
      />
      <path
        d="M22 21.2h8.7l2.6 2.7H22c-1.3 0-2.3 1-2.3 2.3v1.2h-3v-1.6c0-2.5 2-4.6 4.5-4.6Z"
        fill="#f1cf99"
        fillOpacity=".95"
      />

      <path d="M28 30.4 38.6 35 28 39.6v-9.2Z" fill="#081321" fillOpacity=".9" />

      <path
        d="M14.5 22v20.5M14.5 22h5.2M14.5 42.5h5.2"
        stroke="url(#brand-meta)"
        strokeWidth="2.8"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M49.5 22v20.5M44.3 22h5.2M44.3 42.5h5.2"
        stroke="url(#brand-meta)"
        strokeWidth="2.8"
        strokeLinecap="round"
        strokeLinejoin="round"
      />

      {!compact && (
        <>
          <circle cx="43.8" cy="19.5" r="2.4" fill="#c8924d" />
          <path d="M40.5 19.5h-6.3" stroke="#c8924d" strokeWidth="2.2" strokeLinecap="round" />
        </>
      )}
    </svg>
  )
}
