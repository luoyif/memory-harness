import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ActivityCalendar, ActivityCalendarData } from './ActivityCalendar'

describe('ActivityCalendar',()=>{
  afterEach(()=>{cleanup();vi.unstubAllGlobals()})
  it('shows numerical output, task markers and an accessible day detail',()=>{
    const data:ActivityCalendarData={project_id:'p1',timezone:'CST',utc_offset:'+08:00',from:'2026-08-23',until:'2026-08-24',definition:'输出量 = 关键信息 + 会话复盘 + 长期记忆',days:[
      {date:'2026-08-23',evidence:1,knowledge_units:2,episodes:1,memories:1,output:4,tasks_due:0,tasks_done:1,tasks:[]},
      {date:'2026-08-24',evidence:3,knowledge_units:5,episodes:1,memories:2,output:8,tasks_due:1,tasks_done:0,tasks:[{task_id:'t1',title:'完成首页验收',status:'in_progress',priority:2}]},
    ]}
    render(<ActivityCalendar data={data}/>)
    expect(screen.getByRole('heading',{name:/每天留下了多少可用记忆/})).toBeInTheDocument()
    const day=screen.getByRole('gridcell',{name:/8，导入原材料 3，到期待办 1/})
    fireEvent.click(day)
    expect(screen.getByText('完成首页验收')).toBeInTheDocument()
    expect(screen.getByText('正在处理')).toBeInTheDocument()
    expect(screen.getByLabelText('颜色图例')).toHaveTextContent('最高 8')
  })

  it('drops older weeks instead of scrolling and keeps the latest day visible',async()=>{
    vi.stubGlobal('ResizeObserver',class {
      callback:ResizeObserverCallback
      constructor(callback:ResizeObserverCallback){this.callback=callback}
      observe(){this.callback([{contentRect:{width:52}} as ResizeObserverEntry],this as unknown as ResizeObserver)}
      disconnect(){}
      unobserve(){}
    })
    const days:ActivityCalendarData['days']=Array.from({length:28},(_,index)=>({
      date:`2026-08-${String(index+1).padStart(2,'0')}`,evidence:0,knowledge_units:0,episodes:0,memories:0,output:index,tasks_due:0,tasks_done:0,tasks:[],
    }))
    render(<ActivityCalendar data={{project_id:'p1',timezone:'CST',utc_offset:'+08:00',from:'2026-08-01',until:'2026-08-28',definition:'输出量',days}}/>)
    await waitFor(()=>expect(screen.queryByRole('gridcell',{name:/8月1日/})).not.toBeInTheDocument())
    expect(screen.getByRole('gridcell',{name:/8月28日/})).toBeInTheDocument()
    expect(screen.getByRole('grid')).toHaveAccessibleName(/最右侧为当前周/)
  })
})
