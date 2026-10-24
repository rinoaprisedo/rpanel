import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import Modal from '../../components/ui/Modal'
import Input from '../../components/ui/Input'
import Select from '../../components/ui/Select'
import Button from '../../components/ui/Button'
import { menuSchema } from '../../utils/validation'

function MenuFormModal({ open, onClose, onSubmit, initialData, sections = [], loading }) {
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm({ resolver: zodResolver(menuSchema) })

  useEffect(() => {
    if (open) {
      reset(
        initialData
          ? {
              name: initialData.name,
              icon: initialData.icon || '',
              path: initialData.path,
              menu_section_uuid: initialData.menu_section_uuid || '',
              order: initialData.order ?? 0,
              is_active: initialData.is_active ?? true,
            }
          : { name: '', icon: '', path: '', menu_section_uuid: '', order: 0, is_active: true },
      )
    }
  }, [open, initialData, reset])

  return (
    <Modal open={open} onClose={onClose} title={initialData ? 'Edit Menu' : 'Add Menu'}>
      <form className="flex flex-col gap-4" onSubmit={handleSubmit(onSubmit)}>
        <Input label="Name" error={errors.name?.message} {...register('name')} />
        <Input label="Icon (lucide name)" placeholder="e.g. Users" error={errors.icon?.message} {...register('icon')} />
        <Input label="Path" placeholder="/users" error={errors.path?.message} {...register('path')} />
        <Select label="Section" error={errors.menu_section_uuid?.message} {...register('menu_section_uuid')}>
          <option value="">Select section</option>
          {sections.map((s) => (
            <option key={s.uuid} value={s.uuid}>
              {s.name}
            </option>
          ))}
        </Select>
        <Input label="Order" type="number" error={errors.order?.message} {...register('order', { valueAsNumber: true })} />
        <label className="flex items-center gap-2 text-sm text-text-primary">
          <input type="checkbox" className="h-4 w-4 accent-primary" {...register('is_active')} />
          Active
        </label>
        <div className="flex justify-end gap-2 pt-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={loading}>
            {loading ? 'Saving...' : 'Save'}
          </Button>
        </div>
      </form>
    </Modal>
  )
}

export default MenuFormModal
