import type { ButtonHTMLAttributes } from "react";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant;
};

const variantClasses: Record<ButtonVariant, string> = {
  primary: "border-transparent bg-accent text-on-accent hover:bg-accent-strong",
  secondary: "border-line bg-surface text-ink hover:bg-canvas",
  ghost: "border-transparent bg-transparent text-ink hover:bg-canvas",
  danger: "border-transparent bg-danger text-on-danger hover:brightness-95",
};

export function Button({
  variant = "secondary",
  className,
  type = "button",
  ...props
}: ButtonProps): React.ReactElement {
  return (
    <button
      className={[
        "inline-flex min-h-9 items-center justify-center gap-2 rounded-md border",
        "px-4 py-2 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-60",
        variantClasses[variant],
        className,
      ].join(" ")}
      type={type}
      {...props}
    />
  );
}
