import Link from 'next/link';
import type { ComponentProps, ReactNode } from 'react';
import { cn } from '@/lib/utils';
import { Button as UiButton, buttonVariants } from '@/components/ui/button';
import { Spinner } from './spinner';

/* The app-level button API on top of shadcn's Button: the names pages use map onto shadcn variants. */
export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'danger-outline';
export type ButtonSize = 'sm' | 'md' | 'lg' | 'icon' | 'icon-sm';

const variantMap = {
  primary: { variant: 'default' },
  secondary: { variant: 'outline' },
  ghost: { variant: 'ghost' },
  danger: { variant: 'destructive' },
  'danger-outline': {
    variant: 'outline',
    className: 'border-destructive/40 text-destructive hover:bg-destructive/10 hover:text-destructive dark:border-destructive/50',
  },
} as const;

const sizeMap = { sm: 'sm', md: 'default', lg: 'lg', icon: 'icon', 'icon-sm': 'icon-sm' } as const;

export function buttonClass(variant: ButtonVariant = 'secondary', size: ButtonSize = 'md', className?: string) {
  const v = variantMap[variant];
  return cn(buttonVariants({ variant: v.variant, size: sizeMap[size] }), 'className' in v && v.className, className);
}

export interface ButtonProps extends ComponentProps<'button'> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
  icon?: ReactNode;
}

export function Button({ variant = 'secondary', size = 'md', loading, icon, className, children, disabled, type = 'button', ...props }: ButtonProps) {
  const v = variantMap[variant];
  return (
    <UiButton
      type={type}
      variant={v.variant}
      size={sizeMap[size]}
      disabled={disabled || loading}
      className={cn('className' in v && v.className, className)}
      {...props}
    >
      {loading ? <Spinner className="size-3.5" /> : icon}
      {children}
    </UiButton>
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
