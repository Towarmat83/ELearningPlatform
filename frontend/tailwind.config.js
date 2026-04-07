/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/**/*.{html,js,svelte,ts}'],
  theme: {
    extend: {
      colors: {
        primary: {
          50: '#f0f4ff',
          100: '#e0e9ff',
          500: '#4f6ef7',
          600: '#3b52e8',
          700: '#2d3dd6',
          900: '#1a237e',
        },
        ctf: {
          500: '#10b981',
          600: '#059669',
        },
        danger: '#ef4444',
      },
    },
  },
  plugins: [],
};
