/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        brand: {
          50: '#f5f7ff',
          100: '#ebf0fe',
          200: '#ced9fd',
          300: '#adc0fc',
          400: '#8da8fa',
          500: '#6d8ff9',
          600: '#4f72f2',
          700: '#3a59d9',
          800: '#2841bc',
          900: '#1a2b9a',
          DEFAULT: '#4f72f2',
        }
      },
      boxShadow: {
        'soft': '0 2px 15px -1px rgba(0, 0, 0, 0.05), 0 1px 6px -1px rgba(0, 0, 0, 0.02)',
        'premium': '0 10px 30px -5px rgba(0, 0, 0, 0.04), 0 4px 12px -2px rgba(0, 0, 0, 0.03)',
      },
      borderRadius: {
        '2xl': '1.25rem',
        '3xl': '1.75rem',
      },
      fontFamily: {
        sans: ['Inter', 'Pretendard', 'system-ui', 'sans-serif'],
      },
      keyframes: {
        'alert-pulse': {
          '0%, 100%': { boxShadow: '0 0 0 0 rgba(244,63,94,0.45)' },
          '50%':      { boxShadow: '0 0 0 8px rgba(244,63,94,0)' },
        },
      },
      animation: {
        'alert-pulse': 'alert-pulse 2s ease-in-out infinite',
      },
    },
  },
  plugins: [],
}
