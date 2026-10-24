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
          bg: '#ffffff',
          text: '#6b6a65',
          active: 'rgb(var(--color-primary-rgb) / <alpha-value>)',
        },
        surface: {
          bg: '#f7f6f3',
          card: '#ffffff',
          hover: '#f0efe9',
          border: '#e2e1db',
          'border-hover': '#cccbc3',
        },
        text: {
          primary: '#1a1917',
          secondary: '#6b6a65',
          tertiary: '#a09e98',
        },
        danger: '#c0392b',
        'danger-bg': '#fdf0ef',
        success: '#2d6a4f',
        'success-bg': '#eaf4ee',
        warning: '#92400e',
        'warning-bg': '#fef3e2',
        info: '#1d4ed8',
        'info-bg': '#eff6ff',
      },
    },
  },
  plugins: [],
}
