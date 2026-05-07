import { useEffect, useRef, useState } from 'react'
import { ArrowUp } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface BackToTopButtonProps {
  containerSelector?: string
  threshold?: number
  className?: string
}

const DEFAULT_CONTAINER_SELECTOR = '[data-main-scroll-container="true"]'

export default function BackToTopButton({
  containerSelector = DEFAULT_CONTAINER_SELECTOR,
  threshold = 300,
  className = 'fixed right-6 bottom-24 z-50 rounded-full shadow-lg lg:bottom-6',
}: BackToTopButtonProps) {
  const [visible, setVisible] = useState(false)
  const containerRef = useRef<HTMLElement | null>(null)

  useEffect(() => {
    const container = document.querySelector<HTMLElement>(containerSelector)
    if (!container) return
    containerRef.current = container

    const updateVisible = () => {
      setVisible(container.scrollTop > threshold)
    }

    updateVisible()
    container.addEventListener('scroll', updateVisible, { passive: true })

    return () => {
      container.removeEventListener('scroll', updateVisible)
    }
  }, [containerSelector, threshold])

  if (!visible) return null

  return (
    <Button
      size="icon"
      onClick={() => containerRef.current?.scrollTo({ top: 0, behavior: 'smooth' })}
      className={className}
      aria-label="返回顶部"
    >
      <ArrowUp className="size-5" />
    </Button>
  )
}
