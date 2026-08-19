import { Layout, Menu } from 'antd'
import {
  DashboardOutlined,
  FundOutlined,
  LineChartOutlined,
  SwapOutlined,
} from '@ant-design/icons'
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'

import DashboardPage from './pages/Dashboard'
import StockPoolPage from './pages/StockPool'
import StrategyPage from './pages/Strategy'
import TradePage from './pages/Trade'

const { Header, Content } = Layout

const menuItems = [
  { key: '/dashboard', icon: <DashboardOutlined />, label: '首页' },
  { key: '/stocks', icon: <FundOutlined />, label: '股票池' },
  { key: '/strategy', icon: <LineChartOutlined />, label: '策略' },
  { key: '/trade', icon: <SwapOutlined />, label: '模拟交易' },
]

export default function App() {
  const navigate = useNavigate()
  const location = useLocation()

  const selectedKey =
    menuItems.find((m) => location.pathname.startsWith(m.key))?.key ?? '/dashboard'

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{ display: 'flex', alignItems: 'center' }}>
        <div
          style={{
            color: '#fff',
            fontSize: 18,
            fontWeight: 600,
            marginRight: 48,
            whiteSpace: 'nowrap',
          }}
        >
          Quant Dashboard
        </div>
        <Menu
          theme="dark"
          mode="horizontal"
          selectedKeys={[selectedKey]}
          items={menuItems}
          onClick={({ key }) => navigate(key)}
          style={{ flex: 1, minWidth: 0 }}
        />
      </Header>
      <Content style={{ padding: 24 }}>
        <Routes>
          <Route path="/" element={<Navigate to="/dashboard" replace />} />
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/stocks" element={<StockPoolPage />} />
          <Route path="/strategy" element={<StrategyPage />} />
          <Route path="/trade" element={<TradePage />} />
        </Routes>
      </Content>
    </Layout>
  )
}
