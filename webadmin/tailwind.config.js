/** @type {import('tailwindcss').Config} */
export default {
  darkMode: 'class',
  content: ['./index.html', './src/**/*.{js,jsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['DM Sans', 'sans-serif'],
        mono: ['DM Mono', 'monospace'],
      },
      borderRadius: {
        md: '10px',
        lg: '14px',
        xl: '18px',
      },
      boxShadow: {
        card: '0 1px 3px rgba(0,0,0,.07)',
        md: '0 4px 16px rgba(0,0,0,.08)',
        toast: '0 4px 20px rgba(0,0,0,.18)',
      },
      colors: {
        primary: {
          DEFAULT: 'rgb(var(--color-primary-rgb) / <alpha-value>)',
          dark: 'rgb(var(--color-primary-dark-rgb) / <alpha-value>)',
        },
        sidebar: {
          bg: 'rgb(var(--c-sidebar-bg) / <alpha-value>)',
          text: 'rgb(var(--c-sidebar-text) / <alpha-value>)',
          active: 'rgb(var(--color-primary-rgb) / <alpha-value>)',
        },
        surface: {
          bg: 'rgb(var(--c-bg) / <alpha-value>)',
          card: 'rgb(var(--c-card) / <alpha-value>)',
          hover: 'rgb(var(--c-hover) / <alpha-value>)',
          border: 'rgb(var(--c-border) / <alpha-value>)',
          'border-hover': 'rgb(var(--c-border-hover) / <alpha-value>)',
        },
        field: 'rgb(var(--c-field) / <alpha-value>)',
        ide: { border: '#2c2d31', bar: '#25262a', muted: '#7d8290' },
        text: {
          primary: 'rgb(var(--c-text) / <alpha-value>)',
          secondary: 'rgb(var(--c-text-2) / <alpha-value>)',
          tertiary: 'rgb(var(--c-text-3) / <alpha-value>)',
        },
        danger: 'rgb(var(--c-danger) / <alpha-value>)',
        'danger-bg': 'rgb(var(--c-danger-bg) / <alpha-value>)',
        success: 'rgb(var(--c-success) / <alpha-value>)',
        'success-bg': 'rgb(var(--c-success-bg) / <alpha-value>)',
        warning: 'rgb(var(--c-warning) / <alpha-value>)',
        'warning-bg': 'rgb(var(--c-warning-bg) / <alpha-value>)',
        info: 'rgb(var(--c-info) / <alpha-value>)',
        'info-bg': 'rgb(var(--c-info-bg) / <alpha-value>)',
      },
    },
  },
  plugins: [],
}
