<template>
  <div class="container">
    <div class="row">
      <dashboardSidebar class="col-md-3 ml-sm-auto"/>
      <main class="col-md-9 ml-sm-auto">
        <nav aria-label="breadcrumb">
          <ol class="breadcrumb">
            <li class="breadcrumb-item">
              <NuxtLink to="/dashboard">داشبورد</NuxtLink>
            </li>
            <li class="breadcrumb-item">
              <NuxtLink to="/dashboard/articles">مقاله ها</NuxtLink>
            </li>
            <li class="breadcrumb-item active" aria-current="page">ویرایش</li>
          </ol>
        </nav>

        <div class="row">
          <div class="col-12 mb-4 mb-lg-0">
            <form class="card" action="#" @submit.prevent="updateArticle()">
              <div class="card-header">ویرایش مقاله</div>
              <div class="card-body">
                <div class="form-floating mb-3">
                  <input :class="{ 'is-invalid': errors.title }" id="title" class="form-control" type="text"
                         placeholder="عنوان مقاله" aria-label="title" v-model="params.title" required>
                  <label for="title">عنوان مقاله</label>
                  <div v-if="errors.title" class="invalid-feedback">
                    {{ errors.title }}
                  </div>
                </div>
                <div class="form-floating mb-3">
                  <textarea :class="{ 'is-invalid': errors.excerpt }" id="excerpt" class="form-control"
                            placeholder="خلاصه مقاله به صورت متن ساده" v-model="params.excerpt" required></textarea>
                  <label for="excerpt">خلاصه محتوا</label>
                  <div v-if="errors.excerpt" class="invalid-feedback">
                    {{ errors.excerpt }}
                  </div>
                </div>

                <div class="mb-3">
                  <rich-editor :class="{ 'is-invalid': errors.body }" v-model="params.body" id="body"
                               class="form-control" placeholder="متن اصلی مقاله"/>
                  <div v-if="errors.body" class="invalid-feedback">
                    {{ errors.body }}
                  </div>
                </div>

                <div class="mb-3">
                  <input :class="{ 'is-invalid': errors.tags }" class="form-control" type="text" placeholder="تگ ها"
                         v-model="params.tags" aria-label="tags">
                  <div v-if="errors.tags" class="invalid-feedback">
                    {{ errors.tags }}
                  </div>
                </div>

                <div class="mb-3">
                  <input :class="{ 'is-invalid': errors.publishedAt }" class="form-control" type="datetime-local"
                         placeholder="تاریخ انتشار" v-model="params.publishedAt" aria-label="publishment date"/>
                  <div v-if="errors.publishedAt" class="invalid-feedback">
                    {{ errors.publishedAt }}
                  </div>
                </div>

                <div>
                  <div @click.prevent="params.showFilePicker=true; params.pickCover=true;" class="image-picker"
                       :style="{ backgroundImage: `url('${ useFilesUrlResolver().resolve(params.cover) }')` }">
                    <small class="title">تصویر اصلی</small>
                    <div class="body">
                      <small class="fa fa-plus"></small>
                    </div>
                  </div>
                </div>

                <div>
                  <div @click.prevent="params.showFilePicker=true; params.pickCover=false;" class="image-picker"
                       :style="{ backgroundImage: `url('${ useFilesUrlResolver().resolve(params.video) }')` }">
                    <small class="title">ویدئو</small>
                    <div class="body">
                      <small class="fa fa-plus"></small>
                    </div>
                  </div>
                </div>

              </div>
              <div class="card-footer">
                <button :disabled="params.loading" type="submit" class="btn btn-primary rounded submit px-3">
                  <span v-if="!params.loading">ذخیره کن</span>
                  <div v-else class="spinner-border" role="status">
                    <span class="visually-hidden">Loading...</span>
                  </div>
                </button>
              </div>
            </form>
          </div>
        </div>

        <dashboardFileManager modal selectable :show="params.showFilePicker" @close="params.showFilePicker=false"
                              @select="selectFile"/>
      </main>
    </div>
  </div>
</template>

<script lang="ts" setup>
definePageMeta({
  layout: 'dashboard',
})

useHead({
  name: "افزودن مقاله"
})

// article uuid
const {uuid} = useRoute().params

// reflects form parameters
const params = reactive({
  title: null,
  body: null,
  tags: null,
  publishedAt: null,
  cover: null,
  video: null,
  loading: false,
  showFilePicker: false,
  pickCover: false,
})

// reflects the validation errors to corresponding html input.
const errors = reactive({
  title: null,
  body: null,
  tags: null,
  publishedAt: null,
  cover: null,
})

await showArticle()

function tags() {
  if ((typeof params.tags === 'string' || params.tags instanceof String) && (params.tags.length > 0)) {
    return params.tags.split(',')
  }

  return []
}

function selectFile(uuids: string[]) {
  params.showFilePicker = false

  if (uuids && uuids.length == 0) {
    return
  }

  if (params.pickCover) {
    params.cover = uuids[0]

    return
  }

  params.video = uuids[0]
}

async function showArticle() {
  const ut = useTime()

  try {
    const data = await useDashboardArticles().show(uuid)

    params.title = data.title
    params.excerpt = data.excerpt
    params.body = data.body
    params.tags = data.tags.join(',')
    params.cover = data.cover

    if (!ut.isZeroDate(data.published_at)) {
      params.publishedAt = ut.toFormat(data.published_at, 'YYYY-MM-DD HH:mm:ss')
    }
  } catch (error) {
    console.log(error)
  }
}

async function updateArticle() {
  params.loading = true

  try {
    await useDashboardArticles().update(
        uuid,
        params.title,
        params.excerpt,
        params.body,
        tags(),
        params.publishedAt ? useTime().toISOString(params.publishedAt) : null,
        params.cover || null,
        params.video || null,
    )
  } catch (error) {
    console.log(error)
  }

  params.loading = false
}
</script>
