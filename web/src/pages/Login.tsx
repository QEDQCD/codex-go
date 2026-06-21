import { useState, type FormEvent } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth } from '../auth'

const features = [
  { icon: 'chat', text: '通过微信远程控制 Codex 会话' },
  { icon: 'verified_user', text: '实时审批工具权限与提问' },
  { icon: 'terminal', text: 'Web 面板管理会话与日志' },
  { icon: 'notifications', text: 'AI 回复与状态推送到微信' },
]

export default function Login() {
  const { user, loading, login } = useAuth()
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  if (!loading && user) {
    return <Navigate to="/" replace />
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setSubmitting(true)
    try {
      await login(username.trim(), password)
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="login-page min-h-screen flex">
      {/* 左侧介绍 */}
      <div className="hidden lg:flex lg:w-[55%] relative overflow-hidden">
        <div className="login-bg absolute inset-0" />
        <div className="login-grid absolute inset-0 opacity-30" />
        <div className="relative z-10 flex flex-col justify-center px-16 xl:px-24">
          <div className="flex items-center gap-3 mb-8">
            <div className="w-12 h-12 rounded-xl bg-secondary/20 border border-secondary/30 flex items-center justify-center">
              <span className="material-symbols-outlined text-secondary text-[28px]">code</span>
            </div>
            <div>
              <h1 className="text-[32px] font-bold text-on-surface tracking-tight">codex-go</h1>
              <p className="text-[14px] text-on-surface-variant">远程编码控制台</p>
            </div>
          </div>

          <p className="text-[20px] leading-relaxed text-on-surface/90 max-w-lg mb-10">
            通过微信机器人接管 Codex，随时随地编码、审批权限、查看 AI 回复。
          </p>

          <ul className="space-y-4 max-w-md">
            {features.map(f => (
              <li key={f.text} className="flex items-center gap-3 text-[15px] text-on-surface-variant">
                <span className="material-symbols-outlined text-secondary text-[20px]">{f.icon}</span>
                {f.text}
              </li>
            ))}
          </ul>

          <p className="mt-16 text-[12px] text-on-surface-variant/60 font-mono">
            Powered by Codex · Go + React
          </p>
        </div>
      </div>

      {/* 右侧登录 */}
      <div className="flex-1 flex items-center justify-center p-6 bg-background relative">
        <div className="login-bg-mobile absolute inset-0 lg:hidden opacity-40" />
        <div className="relative z-10 w-full max-w-[400px]">
          <div className="lg:hidden text-center mb-8">
            <div className="inline-flex items-center gap-2 mb-3">
              <span className="material-symbols-outlined text-secondary text-[32px]">code</span>
              <span className="text-[24px] font-bold text-on-surface">codex-go</span>
            </div>
            <p className="text-[14px] text-on-surface-variant">通过微信远程控制 Codex</p>
          </div>

          <div className="bg-surface-container/80 backdrop-blur-xl border border-outline-variant rounded-2xl p-8 shadow-2xl">
            <h2 className="text-[22px] font-semibold text-on-surface mb-1">欢迎回来</h2>
            <p className="text-[14px] text-on-surface-variant mb-8">登录以访问管理面板</p>

            <form onSubmit={handleSubmit} className="space-y-5">
              <div>
                <label htmlFor="username" className="block text-[13px] text-on-surface-variant mb-2">
                  用户名
                </label>
                <input
                  id="username"
                  type="text"
                  autoComplete="username"
                  value={username}
                  onChange={e => setUsername(e.target.value)}
                  className="w-full px-4 py-3 rounded-lg bg-surface-container-low border border-outline-variant text-on-surface text-[14px] outline-none focus:border-secondary/60 transition-colors"
                  placeholder="admin"
                  required
                />
              </div>

              <div>
                <label htmlFor="password" className="block text-[13px] text-on-surface-variant mb-2">
                  密码
                </label>
                <input
                  id="password"
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={e => setPassword(e.target.value)}
                  className="w-full px-4 py-3 rounded-lg bg-surface-container-low border border-outline-variant text-on-surface text-[14px] outline-none focus:border-secondary/60 transition-colors"
                  placeholder="请输入密码"
                  required
                />
              </div>

              {error && (
                <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-error/10 border border-error/20 text-error text-[13px]">
                  <span className="material-symbols-outlined text-[18px]">error</span>
                  {error}
                </div>
              )}

              <button
                type="submit"
                disabled={submitting || loading}
                className="w-full py-3 rounded-lg bg-secondary text-on-secondary font-medium text-[15px] hover:bg-secondary/90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              >
                {submitting ? '登录中...' : '登录'}
              </button>
            </form>

            <p className="mt-6 text-center text-[12px] text-on-surface-variant/70">
              默认账号 admin / admin123，可在 config.json 中修改
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
