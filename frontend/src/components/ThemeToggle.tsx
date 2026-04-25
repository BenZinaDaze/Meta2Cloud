import { useTheme } from "next-themes"
import { Moon, Sun, Palette } from "lucide-react"
import { Button } from "@/components/ui/button"

const themes = [
  { value: "light", label: "亮色", icon: Sun },
  { value: "dark", label: "暗色", icon: Moon },
  { value: "warm", label: "彩色", icon: Palette },
] as const

export function ThemeToggle() {
  const { theme, setTheme } = useTheme()
  const currentIndex = themes.findIndex(t => t.value === theme)
  const nextTheme = themes[(currentIndex + 1) % themes.length]
  const Icon = themes.find(t => t.value === theme)?.icon ?? Palette

  return (
    <Button
      variant="ghost"
      size="icon"
      onClick={() => setTheme(nextTheme.value)}
      title={`当前：${themes.find(t => t.value === theme)?.label}，点击切换`}
    >
      <Icon className="size-4" />
      <span className="sr-only">切换主题</span>
    </Button>
  )
}
