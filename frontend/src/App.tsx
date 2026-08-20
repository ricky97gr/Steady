import { Layout, Menu } from 'antd'
import {
  BarChartOutlined,
  DashboardOutlined,
  FundOutlined,
  LineChartOutlined,
  ScheduleOutlined,
  SettingOutlined,
  SwapOutlined,
  RiseOutlined,
} from '@ant-design/icons'
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'

import DashboardPage from './pages/Dashboard'
import StockPoolPage from './pages/StockPool'
import StockDetailPage from './pages/StockDetail'
import StrategyPage from './pages/Strategy'
import TradePage from './pages/Trade'
import BacktestPage from './pages/Backtest'
import MorningBriefPage from './pages/MorningBrief'
import SettingsPage from './pages/Settings'

const { Sider, Content } = Layout

const menuItems = [
  { key: '/dashboard', icon: <DashboardOutlined />, label: '数据总览' },
  { key: '/brief', icon: <ScheduleOutlined />, label: '早盘简报' },
  { key: '/stocks', icon: <FundOutlined />, label: '股票池' },
  { key: '/strategy', icon: <LineChartOutlined />, label: '策略' },
  { key: '/trade', icon: <SwapOutlined />, label: '模拟交易' },
  { key: '/backtest', icon: <BarChartOutlined />, label: '策略回测' },
  { key: '/settings', icon: <SettingOutlined />, label: '设置' },
]

const pageTitles: Record<string, string> = {
  '/dashboard': '数据总览',
  '/brief': '早盘简报',
  '/stocks': '股票池',
  '/strategy': '策略',
  '/trade': '模拟交易',
  '/backtest': '策略回测',
  '/settings': '设置',
}

export default function App() {
  const navigate = useNavigate()
  const location = useLocation()

  const selectedKey =
    menuItems.find((m) => location.pathname.startsWith(m.key))?.key ?? '/dashboard'

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider width={260} className="app-sider" theme="dark">
        <div className="app-logo">
          <div className="logo-icon">
            <RiseOutlined />
          </div>
          <div>
            <div className="logo-text">STEADY 量化</div>
            <div className="logo-sub">个人 A 股量化交易系统</div>
          </div>
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selectedKey]}
          items={menuItems}
          onClick={({ key }) => navigate(key)}
          style={{ background: 'transparent', border: 'none', marginTop: 12 }}
        />
      </Sider>
      <Content style={{ padding: '24px 28px' }}>
        <div className="page-head">
          <h2>{pageTitles[selectedKey] ?? '数据总览'}</h2>
          <span>Steady Quant · V1.0</span>
        </div>
        <div style={{ height: 16 }} />
        <Routes>
          <Route path="/" element={<Navigate to="/dashboard" replace />} />
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/brief" element={<MorningBriefPage />} />
          <Route path="/stocks" element={<StockPoolPage />} />
          <Route path="/stocks/:code" element={<StockDetailPage />} />
          <Route path="/strategy" element={<StrategyPage />} />
          <Route path="/trade" element={<TradePage />} />
          <Route path="/backtest" element={<BacktestPage />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Routes>
      </Content>
    </Layout>
  )
}
