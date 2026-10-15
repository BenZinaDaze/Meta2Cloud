import { Toaster as Sonner } from "sonner"

type ToasterProps = React.ComponentProps<typeof Sonner>

const Toaster = ({ ...props }: ToasterProps) => {
  return (
    <Sonner
      closeButton
      position="top-right"
      theme="dark"
      toastOptions={{
        style: {
          background: 'oklch(20% 0.014 220)',
          color: 'oklch(96% 0.008 200)',
          border: '1px solid oklch(100% 0 0 / 0.1)',
          borderRadius: '0.875rem',
          boxShadow: '0 16px 48px -12px oklch(5% 0.01 140 / 0.9)',
        },
        classNames: {
          toast:
            "group toast group-[.toaster]:text-foreground group-[.toaster]:backdrop-blur-xl",
          description: "group-[.toast]:text-muted-foreground",
          actionButton:
            "group-[.toast]:bg-primary group-[.toast]:text-primary-foreground",
          cancelButton:
            "group-[.toast]:bg-muted group-[.toast]:text-muted-foreground",
          success: "[&_[data-icon]]:text-success",
          error: "[&_[data-icon]]:text-destructive",
          warning: "[&_[data-icon]]:text-warning",
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
