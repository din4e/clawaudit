import { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@/hooks/redux';
import { selectTheme, toggleTheme, setTheme } from '@/store/slices/themeSlice';

/**
 * Hook for theme management
 */
export function useTheme() {
  const dispatch = useAppDispatch();
  const theme = useAppSelector(selectTheme);

  useEffect(() => {
    // Apply theme to document on mount and when theme changes
    if (typeof document !== 'undefined') {
      if (theme === 'light') {
        document.documentElement.setAttribute('data-theme', 'light');
      } else {
        document.documentElement.removeAttribute('data-theme');
      }
    }
  }, [theme]);

  return {
    theme,
    toggleTheme: () => dispatch(toggleTheme()),
    setTheme: (newTheme: 'light' | 'dark') => dispatch(setTheme(newTheme)),
    isLight: theme === 'light',
    isDark: theme === 'dark',
  };
}
