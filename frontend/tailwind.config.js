/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // 品牌主色阶对齐官网「纸·墨·金」方案 B+：
        // - 50/100/200 与 900/950 采用暖石/深海中性面，避免 100+ 处 bg-primary-50/100 提示块与 amber 告警黄混淆
        // - 300/400/500/600 保留克制琥珀金与铜金（焦点环、链接与细锚点），700 为暖炭墨确保浅底高对比
        primary: {
          50: '#f5f4f0',
          100: '#eceae4',
          200: '#dddad2',
          300: '#f5ca64',
          400: '#e8a825',
          500: '#d49217',
          600: '#8a5f0a',
          700: '#201f1a',
          800: '#2c3b55',
          900: '#1c2638',
          950: '#151e2f'
        },
        // 辅助色 - 暖灰/深海灰
        accent: {
          50: '#fafaf8',
          100: '#f3f2ee',
          200: '#e7e5e0',
          300: '#d8d5cf',
          400: '#94a3b8',
          500: '#6e6b62',
          600: '#454339',
          700: '#2c3b55',
          800: '#101724',
          900: '#070b14',
          950: '#0a0f1a'
        },
        // 仅暖化 50–300（浅色纸底与发丝边）；400–950 保持 Tailwind 默认冷灰，
        // 避免上游 27 个文件 129 处 dark:bg-gray-700/800/900 在黑曜蓝底上发褐
        gray: {
          50: '#fafaf8',
          100: '#f3f2ee',
          200: '#e7e5e0',
          300: '#d8d5cf',
          400: '#9ca3af',
          500: '#6b7280',
          600: '#4b5563',
          700: '#374151',
          800: '#1f2937',
          900: '#111827',
          950: '#030712'
        },
        // 深色模式背景：黑曜午夜蓝画布（#0a0f1a）+ 深海蓝卡片（#101724）+ 暗色侧栏（#070b14），与官网及 Studio 同源
        dark: {
          50: '#f8fafc',
          100: '#f1f5f9',
          200: '#e2e8f0',
          300: '#cbd5e1',
          400: '#94a3b8',
          500: '#64748b',
          600: '#2c3b55',
          700: '#1f2b40',
          800: '#101724',
          900: '#070b14',
          950: '#0a0f1a'
        }
      },
      fontFamily: {
        sans: [
          'Plus Jakarta Sans Variable',
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['JetBrains Mono Variable', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: '0 8px 32px rgba(32, 31, 26, 0.08)',
        'glass-sm': '0 4px 16px rgba(32, 31, 26, 0.06)',
        glow: '0 2px 10px rgba(32, 31, 26, 0.08)',
        'glow-lg': '0 4px 16px rgba(32, 31, 26, 0.12)',
        card: '0 1px 2px rgba(32, 31, 26, 0.04)',
        'card-hover': '0 8px 24px rgba(32, 31, 26, 0.07)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.08)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #201f1a 0%, #33312a 100%)',
        'gradient-dark': 'linear-gradient(135deg, #101724 0%, #0a0f1a 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.1) 0%, rgba(255,255,255,0.05) 100%)',
        'mesh-gradient': 'none'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 2px 10px rgba(32, 31, 26, 0.08)' },
          '100%': { boxShadow: '0 4px 16px rgba(32, 31, 26, 0.12)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
