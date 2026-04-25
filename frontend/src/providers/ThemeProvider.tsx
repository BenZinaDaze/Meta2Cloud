import { ThemeProvider as NextThemesProvider } from "next-themes"
import type { ReactNode } from "react"

export function ThemeProvider({ children }: { children: ReactNode }) {
  return (
    <NextThemesProvider
      attribute="class"
      defaultTheme="warm"
      enableSystem={false}
      disableTransitionOnChange
      themes={["light", "dark", "warm"]}
    >
      {children}
    </NextThemesProvider>
  )
}
