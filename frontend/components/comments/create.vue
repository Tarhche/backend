<template>
  <form class="my-3 py-1 d-flex flex-column" :class="{'mt-0': props.isReplying, 'child': props.isReplying}"
        @submit.prevent="createComment()">
    <textarea :disabled="params.pending" v-model.trim="params.body" class="form-control mb-1"
              placeholder="دیدگاه خود را اینجا بنویسید" rows="3" required></textarea>
    <span class="error text-danger" v-if="params.error">متن پیام باید بیشتر باشد.</span>
    <div class="d-flex gap-2">
      <button :disabled="params.pending" :class="{'btn-sm': props.isReplying}" type="submit"
              class="btn btn-success align-self-start my-1">
        <span v-if="!params.pending">ثبت دیدگاه</span>
        <div v-else class="spinner-border" role="status">
          <span class="visually-hidden">Loading...</span>
        </div>
      </button>
    </div>
  </form>
</template>

<script setup lang="ts">
const emit = defineEmits(['commentCreated'])

const props = defineProps({
  objectType: {
    type: String,
    required: true
  },
  objectUUID: {
    type: String,
    required: true
  },
  parentUUID: {
    type: String,
    default: "",
  },
  isReplying: {
    type: Boolean,
    default: false
  }
})

const params = reactive({
  pending: false,
  error: false,
  body: "",
})

async function createComment() {
  const body = {
    object_uuid: props.objectUUID,
    object_type: props.objectType,
    parent_uuid: props.parentUUID,
    body: params.body,
  }

  try {
    params.pending = true;
    const data = await useUser().$fetch(useApiUrlResolver().resolve('api/comments'), {
      method: 'POST',
      headers: {authorization: `Bearer ${useAuth().accessToken()}`},
      body: body,
    })

    emit('commentCreated', data)
    params.body = ""
  } catch (e) {
    params.error = true;
    console.log(e)
  }
  params.pending = false;
}
</script>
