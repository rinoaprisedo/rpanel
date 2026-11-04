// WinRAR-style archive icon (a stack of three books tied with a strap). lucide has no such icon;
// it is multi-colored, so it ignores the stroke/fill props the lucide icons receive.
function ArchiveIcon({ size = 16, className, title, 'aria-label': ariaLabel }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" className={className} role="img" aria-label={ariaLabel || title}>
      {title && <title>{title}</title>}
      <rect x="2.5" y="15" width="19" height="6" rx="1.4" fill="#7c3aed" />
      <rect x="3.5" y="9" width="17" height="6" rx="1.4" fill="#2563eb" />
      <rect x="4.5" y="3" width="15" height="6" rx="1.4" fill="#dc2626" />
      <rect x="10.5" y="3" width="3" height="18" fill="#fff" fillOpacity=".85" />
    </svg>
  )
}

export default ArchiveIcon
