import type { ThemeConfig } from 'antd'

// 全局主题：深蓝金融风
export const themeConfig: ThemeConfig = {
  token: {
    colorPrimary: '#1d39c4',
    colorInfo: '#1d39c4',
    colorSuccess: '#52c41a',
    colorWarning: '#faad14',
    colorError: '#f5222d',
    colorText: '#1f2d3d',
    colorTextSecondary: '#66768b',
    colorBgLayout: '#f2f5fb',
    colorBorderSecondary: '#e6ecf5',
    borderRadius: 10,
    borderRadiusLG: 14,
    fontFamily:
      "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
    boxShadowSecondary: '0 6px 24px rgba(29, 57, 196, 0.08)',
  },
  components: {
    Card: {
      boxShadowTertiary: '0 2px 12px rgba(31, 45, 61, 0.06)',
      headerBg: 'transparent',
    },
    Table: {
      headerBg: '#f7f9fd',
      headerColor: '#66768b',
      rowHoverBg: '#f5f8ff',
    },
    Menu: {
      itemBg: 'transparent',
    },
  },
}
