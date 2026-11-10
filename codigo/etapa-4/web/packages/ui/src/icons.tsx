// Inline SVG icons: no icon font, nothing loaded from elsewhere.
import type { ComponentPropsWithoutRef } from "react";

type IconProps = Omit<ComponentPropsWithoutRef<"svg">, "children" | "path" | "d">;

function Icon({ d, ...rest }: IconProps & { d: string }) {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="square" aria-hidden="true" focusable="false" {...rest}>
      <path d={d} />
    </svg>
  );
}

export const UploadIcon = (p: IconProps) => <Icon d="M12 16V4M6 10l6-6 6 6M4 20h16" {...p} />;
export const CubeIcon = (p: IconProps) => <Icon d="M12 2l9 5v10l-9 5-9-5V7zM12 22V12M21 7l-9 5-9-5" {...p} />;
export const TruckIcon = (p: IconProps) => <Icon d="M2 5h12v11H2zM14 9h4l4 4v3h-8M6 19a2 2 0 100-4 2 2 0 000 4zM18 19a2 2 0 100-4 2 2 0 000 4z" {...p} />;
export const CheckIcon = (p: IconProps) => <Icon d="M4 12l5 5L20 6" {...p} />;
export const MenuIcon = (p: IconProps) => <Icon d="M3 6h18M3 12h18M3 18h18" {...p} />;
export const CloseIcon = (p: IconProps) => <Icon d="M5 5l14 14M19 5L5 19" {...p} />;
export const ArrowRightIcon = (p: IconProps) => <Icon d="M4 12h16M14 6l6 6-6 6" {...p} />;
export const ShieldIcon = (p: IconProps) => <Icon d="M12 2l8 3v6c0 5-3.5 9-8 11-4.5-2-8-6-8-11V5z" {...p} />;
