import { describe, expect, it } from 'vitest'
import type { ReactNode } from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { db } from '@/mocks/db'
import {
  useCreateProject,
  useCreateTask,
  useDeleteProject,
  useDeleteTask,
  useProject,
  useProjects,
  useTasks,
  useUpdateProject,
  useUpdateTask,
} from './api'

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
  })
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
  return { qc, Wrapper }
}

describe('projects api — projects', () => {
  it('useProjects lists seeded projects with pagination meta', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useProjects({ page: 1, page_size: 10 }), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.total).toBeGreaterThanOrEqual(3)
    expect(result.current.data?.items.some((p) => p.code === 'PRJ-2026-001')).toBe(true)
  })

  it('useProjects filters by status', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useProjects({ status: 'PAUSED' }), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.items).toHaveLength(1)
    expect(result.current.data?.items[0].status).toBe('PAUSED')
  })

  it('useProjects filters by keyword q', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useProjects({ q: '桥梁' }), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.items).toHaveLength(1)
    expect(result.current.data?.items[0].code).toBe('PRJ-2026-002')
  })

  it('useCreateProject stores custom_fields and useProject reads them back', async () => {
    const { Wrapper } = makeWrapper()
    const create = renderHook(() => useCreateProject(), { wrapper: Wrapper })
    const created = await create.result.current.mutateAsync({
      code: 'PRJ-NEW-001',
      name: '新建测试项目',
      status: 'ACTIVE',
      custom_fields: { region: '西湖区', budget: 88 },
    })
    expect(created.code).toBe('PRJ-NEW-001')
    expect(created.custom_fields).toEqual({ region: '西湖区', budget: 88 })
    expect(db.projects.some((p) => p.code === 'PRJ-NEW-001')).toBe(true)

    const read = renderHook(() => useProject(created.id), { wrapper: Wrapper })
    await waitFor(() => expect(read.result.current.isSuccess).toBe(true))
    expect(read.result.current.data?.custom_fields).toEqual({ region: '西湖区', budget: 88 })
  })

  it('useUpdateProject edits a project', async () => {
    const { Wrapper } = makeWrapper()
    const update = renderHook(() => useUpdateProject(), { wrapper: Wrapper })
    const updated = await update.result.current.mutateAsync({
      id: 1,
      body: { status: 'ARCHIVED', name: '滨江绿道巡查（归档）' },
    })
    expect(updated.status).toBe('ARCHIVED')
    expect(updated.name).toBe('滨江绿道巡查（归档）')
  })

  it('useDeleteProject rejects deleting a project with inspections (409 CONFLICT)', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useDeleteProject(), { wrapper: Wrapper })
    // project 1 has seeded inspections → RESTRICT
    await expect(result.current.mutateAsync(1)).rejects.toMatchObject({ code: 'CONFLICT' })
    expect(db.projects.some((p) => p.id === 1)).toBe(true)
  })

  it('useDeleteProject removes a project without inspections', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useDeleteProject(), { wrapper: Wrapper })
    // project 3 (ARCHIVED) has no inspections
    await result.current.mutateAsync(3)
    expect(db.projects.some((p) => p.id === 3)).toBe(false)
  })
})

describe('projects api — tasks', () => {
  it('useTasks lists tasks for a project and filters by status', async () => {
    const { Wrapper } = makeWrapper()
    const all = renderHook(() => useTasks({ project_id: 1 }), { wrapper: Wrapper })
    await waitFor(() => expect(all.result.current.isSuccess).toBe(true))
    expect(all.result.current.data?.items.length).toBeGreaterThanOrEqual(2)

    const pending = renderHook(() => useTasks({ project_id: 1, status: 'PENDING' }), {
      wrapper: Wrapper,
    })
    await waitFor(() => expect(pending.result.current.isSuccess).toBe(true))
    expect(pending.result.current.data?.items.every((tk) => tk.status === 'PENDING')).toBe(true)
  })

  it('useTasks filters by assignee_id', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useTasks({ project_id: 1, assignee_id: 3 }), {
      wrapper: Wrapper,
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.items.every((tk) => tk.assignee_id === 3)).toBe(true)
  })

  it('useCreateTask, useUpdateTask and useDeleteTask mutate the tasks store', async () => {
    const { Wrapper } = makeWrapper()
    const create = renderHook(() => useCreateTask(), { wrapper: Wrapper })
    const created = await create.result.current.mutateAsync({
      project_id: 1,
      title: '新任务',
      status: 'PENDING',
      assignee_id: 2,
    })
    expect(created.status).toBe('PENDING')
    expect(created.assignee_id).toBe(2)

    const update = renderHook(() => useUpdateTask(), { wrapper: Wrapper })
    const updated = await update.result.current.mutateAsync({
      id: created.id,
      body: { status: 'COMPLETED' },
    })
    expect(updated.status).toBe('COMPLETED')

    const remove = renderHook(() => useDeleteTask(), { wrapper: Wrapper })
    await remove.result.current.mutateAsync(created.id)
    expect(db.tasks.some((tk) => tk.id === created.id)).toBe(false)
  })
})
