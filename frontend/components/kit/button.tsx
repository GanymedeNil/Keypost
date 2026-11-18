import Link from 'next/link';
import type { ComponentProps, ReactNode } from 'react';
import { cn } from '@/lib/utils';
import { Spinner } from './spinner';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'danger-outline';
export type ButtonSize = 'sm' | 'md' | 'lg' | 'icon' | 'icon-sm';

const variants: Record<ButtonVariant, string> = {
  primary: 'bg-accent text-accent-fg hover:bg-accent-hover shadow-card',
  secondary: 'border border-line-strong bg-surface text-fg hover:bg-surface-2 shadow-card',
  ghost: 'text-muted hover:bg-surface-2 hover:text-fg',
  danger: 'bg-danger text-white hover:opacity-90 shadow-card',
  'danger-outline': 'border border-danger/40 bg-surface text-danger-text hover:bg-danger-soft',
};

const sizes: Record<ButtonSize, string> = {
  sm: 'h-7 gap-1 rounded-md px-2.5 text-xs',
  md: 'h-8 gap-1.5 rounded-md px-3 text-sm',
  lg: 'h-10 gap-2 rounded-lg px-4 text-sm',
  icon: 'size-8 rounded-md',
  'icon-sm': 'size-7 rounded-md',
};

export function buttonClass(variant: ButtonVariant = 'secondary', size: ButtonSize = 'md', className?: string) {
  return cn(
    'inline-flex shrink-0 select-none items-center justify-center whitespace-nowrap font-medium transition-colors',
    'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent',
    'disabled:pointer-events-none disabled:opacity-50 aria-disabled:pointer-events-none aria-disabled:opacity-50',
    variants[variant],
    sizes[size],
    className,
  );
}

export interface ButtonProps extends ComponentProps<'button'> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
  icon?: ReactNode;
}

export function Button({ variant = 'secondary', size = 'md', loading, icon, className, children, disabled, type = 'button', ...props }: ButtonProps) {
  return (
    <button type={type} disabled={disabled || loading} className={buttonClass(variant, size, className)} {...props}>
      {loading ? <Spinner className="size-3.5" /> : icon}
      {children}
    </button>
  );
}

export interface ButtonLinkProps extends Omit<ComponentProps<typeof Link>, 'className'> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  className?: string;
  icon?: ReactNode;
}

export function ButtonLink({ variant = 'secondary', size = 'md', className, icon, children, ...props }: ButtonLinkProps) {
  return (
    <Link className={buttonClass(variant, size, className)} {...props}>
      {icon}
      {children}
    </Link>
  );
}

/** Plain anchor styled as a button (downloads, external links, API-served files). */
export function ButtonAnchor({
  variant = 'secondary',
  size = 'md',
  className,
  icon,
  children,
  ...props
}: Omit<ComponentProps<'a'>, 'className'> & { variant?: ButtonVariant; size?: ButtonSize; className?: string; icon?: ReactNode }) {
  return (
    <a className={buttonClass(variant, size, className)} {...props}>
      {icon}
      {children}
    </a>
  );
}
