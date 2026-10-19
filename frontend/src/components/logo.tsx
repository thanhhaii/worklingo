export function Logo({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 40 40" className={className} aria-hidden>
      <rect x="2" y="2" width="36" height="36" rx="9" fill="#3F0E40" />
      <rect x="9" y="12" width="22" height="14" rx="4" fill="#fff" />
      <path d="M14 26 L12 31 L19 26 Z" fill="#fff" />
      <circle cx="15" cy="19" r="1.8" fill="#E01E5A" />
      <circle cx="20" cy="19" r="1.8" fill="#2EB67D" />
      <circle cx="25" cy="19" r="1.8" fill="#ECB22E" />
    </svg>
  );
}
