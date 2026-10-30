export default function Logo({ className = 'size-8' }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 64 64"
      aria-hidden="true"
      className={className}
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      <defs>
        <linearGradient id="tide-mark-bg" x1="8" y1="6" x2="58" y2="60" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor="#4cd16f" />
          <stop offset="1" stopColor="#169e4c" />
        </linearGradient>
        <radialGradient id="tide-mark-glow" cx="0" cy="0" r="1" gradientUnits="userSpaceOnUse" gradientTransform="translate(18 12) rotate(65) scale(32 28)">
          <stop stopColor="oklch(95% 0.08 135)" stopOpacity=".5" />
          <stop offset="1" stopColor="oklch(95% 0.08 135)" stopOpacity="0" />
        </radialGradient>
      </defs>

      <rect x="4" y="4" width="56" height="56" rx="17" fill="url(#tide-mark-bg)" />
      <rect x="4" y="4" width="56" height="56" rx="17" fill="url(#tide-mark-glow)" />
      <rect x="4.75" y="4.75" width="54.5" height="54.5" rx="16.25" stroke="oklch(100% 0 0 / 0.35)" strokeWidth="1.5" />

      {/* 潮汐波浪（白色，高对比） */}
      <path
        d="M10 38c5.5 0 5.5-5 11-5s5.5 5 11 5 5.5-5 11-5 5.5 5 11 5"
        stroke="oklch(100% 0 0)"
        strokeWidth={5}
        strokeLinecap="round"
      />
      <path
        d="M13 49c4.75 0 4.75-4.5 9.5-4.5s4.75 4.5 9.5 4.5 4.75-4.5 9.5-4.5 4.75 4.5 9.5 4.5"
        stroke="oklch(100% 0 0)"
        strokeOpacity={0.6}
        strokeWidth={4.4}
        strokeLinecap="round"
      />
      {/* 元数据星点 */}
      <path
        d="M32 10.7 33.3 15.8 40 16.6 36 20.5 37 26 32 23.4 27 26 28 20.5 24 16.6 30.7 15.8Z"
        fill="oklch(100% 0 0)"
      />
    </svg>
  )
}
