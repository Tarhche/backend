<script setup lang="ts">
const data = ref({
  row: [
    {
      row_number: 1,
      image: "",
      author: "مدیر ارشد",
      id: "1",
      header: "هوش مصنوعی",
      date: "1402.2.5",
      status: "فعال",
      action: "",
    },
    {
      row_number: 2,
      image: "",
      author: "مدیر ارشد",
      id: "1",
      header: "هوش مصنوعی",
      date: "1402.2.5",
      status: "فعال",
      action: "",
    },
    {
      row_number: 3,
      image: "",
      author: "مدیر ارشد",
      id: "1",
      header: "هوش مصنوعی",
      date: "1402.2.5",
      status: "فعال",
      action: "",
    },
  ]


})
</script>

<template>
  <div class="articles-list" dir="rtl">
    <div class="header-list d-flex justify-content-between align-items-center mb-2">
      <div class="title-list">لیست اطلاعیه ها</div>
      <div class="create-list px-3 py-1">
        <router-link to="/dashboard/articles/create">ثبت اطلاعیه جدید</router-link>
      </div>
    </div>
    <div class="list-body ">
      <div class="table-responsive-md">
        <table class="table table-striped rounded overflow-hidden align-middle w-100">
          <thead class="table-dark text-center  ">
          <tr class="py-3">
            <th>ردیف</th>
            <th>تصویر</th>
            <th>ناشر</th>
            <th>شناسه</th>
            <th>عنوان</th>
            <th>تاریخ انتشار</th>
            <th>وضعیت</th>
            <th>عملیات</th>
          </tr>
          </thead>
          <tbody class="text-center ">
          <tr v-for="(item,index) in data.row" :key="index"  >
            <th>{{ item.row_number }}</th>
            <td class="" ><img class="img-thumbnail rounded-circle w-50" :src="item.image" :alt="item.header"></td>
            <td>{{ item.author }}</td>
            <td>{{ item.id }}</td>
            <td class="list-header">{{ item.header }}</td>
            <td>{{ item.date }}</td>
            <td><span class="px-3 py-1"> {{ item.status }}</span></td>
            <td class="">
              <span class="mx-md-1 d-block d-lg-inline-block "><i class="fa-solid fa-trash text-danger"></i></span>
              <span class="mx-md-1 d-block d-lg-inline-block "><i class="fa-solid fa-pen text-warning"></i></span>
              <span class="mx-md-1 d-block d-lg-inline-block "><i class="fa-solid fa-eye text-success"></i></span>
            </td>
          </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<style scoped>
thead tr {
  color: #817d7d;
  font-size: 0.9rem;
}
tbody tr {
  color: #817d7d;
  font-size: 0.8rem;
}
.list-header{
  text-overflow: ellipsis;
  overflow: hidden;
  white-space: nowrap;
}
</style>