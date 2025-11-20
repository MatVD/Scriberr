export function SyntheziaLogo({ className = '' }: { className?: string }) {
  return (
    <img
      src="/synthezia-logo.png"
      alt="Synthezia"
      className={`w-auto select-none ${className}`}
    />
  );
}
