<template>
  <div class="card">
    <div class="card-header">کامنت های من</div>
    <div class="card-body">
      <div class="table-responsive">
        <table class="table table-striped table-borderless table-hover align-middle">
          <thead class="border-bottom">
          <tr>
            <th scope="col">#</th>
            <th scope="col">محتوا</th>
            <th scope="col">وضعیت انتشار</th>
            <th scope="col">تاریخ ثبت</th>
            <th scope="col">#</th>
          </tr>
          </thead>
          <tbody v-if="!pending">
          <tr v-for="(comment, index) in data.items" :key="index">
            <th scope="row">{{ index + 1 }}</th>
            <td>{{ trim(comment.body, 25) }}</td>
            <td>
              <span v-if="useTime().isZeroDate(comment.approved_at)" class="fa fa-times text-danger"></span>
              <span class="fa fa-check text-success"></span>
            </td>
            <td>
              <span v-if="useTime().isZeroDate(comment.created_at)" class="fa fa-times text-danger"></span>
              <span v-else>{{ useTime().toAgo(comment.created_at) }}</span>
            </td>
            <td>
              <NuxtLink :to="`/${comment.object_type}s/${comment.object_uuid}`" target="_blank" class="btn mx-1 btn-sm btn-primary">
                <span class="fa fa-eye"></span>
              </NuxtLink>
              <NuxtLink v-if="useTime().isZeroDate(comment.approved_at)" :to="`/dashboard/my/comments/edit/${comment.uuid}`" class="btn mx-1 btn-sm btn-primary">
                <span class="fa fa-pen"></span>
              </NuxtLink>
              <button @click.prevent="deleteComment(comment.uuid)" type="button"
                      class="btn mx-1 btn-sm btn-danger">
                <span class="fa fa-trash"></span>
              </button>
            </td>
          </tr>
          <tr v-if="data.items.length == 0">
            <td colspan="6">
              <p class="m-2">هیچ کامنتی وجود ندارد</p>
            </td>
          </tr>
          </tbody>
        </table>
      </div>
      <p class="text-center">
        <NuxtLink v-if="!pending && data.pagination.total_pages > 1" to="/dashboard/my/comments">مشاهده بیشتر
        </NuxtLink>
      </p>
    </div>
  </div>
</template>

<script lang="ts" setup>
const page = (useRoute().query.page) || 1

function trim(str, maxLength): string {
  if (str.length > maxLength) {
    return str.substring(0, maxLength) + "...";
  }

  return str;
}

const {data, pending, error} = await useAsyncData(
    'dashboard.my.comments.index',
    () => useDashboardMyComments().index(page)
)

async function deleteComment(uuid: string) {
  if (!confirm('آیا میخواهید این کامنت را حذف کنید؟')) {
    return
  }

  await useDashboardMyComments().delete(uuid)

  data.items = data.items.filter((comment) => comment.uuid != uuid)
}
</script>
