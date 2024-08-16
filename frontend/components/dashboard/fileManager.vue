<template>
  <div :class="{ modal: modal, 'd-block': show }" tabindex="-1">
    <div :class="{ 'modal-dialog': modal, 'modal-lg': modal }">
      <div :class="{ 'modal-content': modal, card: !modal }">
        <div v-if="modal" class="modal-header">
          <button @click.prevent="$emit('close')" type="button" class="btn-close"></button>
        </div>
        <div :class="{ 'modal-body': modal, 'card-body': !modal }">
          <label :class="{ active: isSelected(file.uuid) }" class="brick tile-picker"
                 v-for="(file, index) in data.items" :key="index"
                 :style="{backgroundImage: `url('${useFilesUrlResolver().resolve(file.uuid)}')`}">
            <template v-if="selectable">
              <input v-model="data.selected" :value="file.uuid" type="checkbox">
              <i class="tile-checked"></i>
            </template>

            <div class="overlay">
              <div class="actions">
                <button type="button" class="btn btn-sm btn-danger m-1" @click.self.prevent="deleteOne(file.uuid)">
                  <span class="align-middle fa fa-trash"></span>
                </button>
                <a :href="useFilesUrlResolver().resolve(file.uuid)" target="_blank" class="btn btn-sm btn-primary m-1">
                  <span class="align-middle fa fa-eye"></span>
                </a>
              </div>
            </div>
          </label>
          <p v-if="data.error" class="alert alert-danger">{{ data.error }}</p>
          <p v-if="!data.pending && !data.error && data.items.length == 0" class="alert alert-info">هیچ فایلی وجود
            ندارد</p>
        </div>
        <div :class="{ 'modal-footer': modal, 'card-footer': !modal }">
          <label for="file-picker" type="button" class="btn btn-primary m-1">
            <span class="align-middle fa fa-plus m-1"></span>
            <span class="m-1">افزودن فایل</span>
            <input accept="image/*" @change="uploadFile" @click="clearPickedFiles" class="visually-hidden"
                   id="file-picker" type="file" name="file" multiple/>
          </label>
          <button v-if="data.selected.length" @click.prevent="$emit('select', data.selected)" class="btn btn-success">
            <span class="fa fa-check m-1"></span>
            <span class="m-1">تایید</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
defineEmits(['close', 'select'])

const props = defineProps({
  "modal": Boolean,
  "selectable": Boolean,
  "multi": Boolean,
  "show": Boolean,
})

const {modal, selectable} = props

// takes create of data and its related states
const data = reactive({
  pending: false,
  items: [],
  pagination: {
    currentPage: 0,
    totalPages: 0,
  },
  error: null,
  selected: [],
})

await loadData()

function isSelected(uuid: string): Boolean {
  return data.selected.filter((i) => i == uuid).length > 0
}

async function loadData() {
  data.pending = true
  const response = await useAsyncData(
      'dashboard.files.index',
      useDashboardFiles().index
  )

  if (response.error.value != null) {
    data.error = "مشکلی در دریافت اطلاعات به وجود آمده است"

    return
  }

  const responseBody = response.data.value
  const pagination = responseBody.pagination

  data.pending = false
  data.items = responseBody.items

  data.pagination.currentPage = pagination.current_page
  data.pagination.totalPages = pagination.total_pages
}

function clearPickedFiles(event) {
  try {
    event.target.value = '';
    if (event.target.value) {
      event.target.type = "text";
      event.target.type = "file";
    }
  } catch (error) {
    console.log(error)
  }
}

async function deleteOne(uuid: string) {
  if (!confirm('آیا میخواهید این فایل را حذف کنید؟')) {
    return
  }

  await useDashboardFiles().delete(uuid)

  data.value.items = data.value.items.filter(item => item.uuid != uuid)
}

async function uploadFile(event) {
  if (event.target.files.length == 0) {
    return
  }

  await useDashboardFiles().create(event.target.files[0])
  await loadData()
}
</script>