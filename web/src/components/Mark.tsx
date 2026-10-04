export function Mark({ size = 36 }: { size?: number }) {
  return (
    <svg className="siroc-mark" width={size} height={size} viewBox="0 0 32 32" aria-hidden="true">
      <rect width="32" height="32" rx="8" fill="#14110f" />
      <path d="M6 20c4-8 8-10 12-6s6 2 8-4" stroke="#e2783a" strokeWidth="2.2" strokeLinecap="round" fill="none" />
      <path d="M7 24c5-6 9-7 12-3s5 1 7-3" stroke="#f4eadc" strokeWidth="1.6" strokeLinecap="round" fill="none" opacity="0.85" />
    </svg>
  );
}
